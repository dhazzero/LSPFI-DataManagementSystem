package main

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"database/sql/driver"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

type LegacySnapshot struct {
	Catalog LegacyCatalog          `json:"catalog"`
	Rows    map[string][][]*string `json:"rows"`
}
type legacyQueryer interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

// Dedicated connections ensure session settings can never leak to HTTP queries.
func beginArchiveTx(ctx context.Context, db *sql.DB) (*sql.Tx, func(), error) {
	conn, e := db.Conn(ctx)
	if e != nil {
		return nil, nil, e
	}
	tx, e := conn.BeginTx(ctx, nil)
	if e != nil {
		conn.Close()
		return nil, nil, e
	}
	cleanup := func() {
		tx.Rollback()
		resetCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if _, e := conn.ExecContext(resetCtx, "SET SESSION FOREIGN_KEY_CHECKS=1"); e != nil {
			conn.Raw(func(any) error { return driver.ErrBadConn })
		}
		conn.Close()
	}
	return tx, cleanup, nil
}

type LegacyOrphan struct {
	Table      string   `json:"table"`
	Constraint string   `json:"constraint"`
	Columns    []string `json:"columns"`
	Target     string   `json:"target"`
	Count      int64    `json:"count"`
}

func findLegacyOrphans(ctx context.Context, tx *sql.Tx, c LegacyCatalog) ([]LegacyOrphan, error) {
	result := []LegacyOrphan{}
	for _, t := range c.Tables {
		groups := map[string][]LegacyRelation{}
		for _, r := range t.Relations {
			groups[r.Name] = append(groups[r.Name], r)
		}
		names := []string{}
		for name := range groups {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			rels := groups[name]
			conditions := []string{}
			notNull := []string{}
			columns := []string{}
			for _, r := range rels {
				conditions = append(conditions, "p."+quoteID(r.TargetColumn)+"=c."+quoteID(r.Column))
				notNull = append(notNull, "c."+quoteID(r.Column)+" IS NOT NULL")
				columns = append(columns, r.Column)
			}
			query := "SELECT COUNT(*) FROM " + quoteID(t.Name) + " c WHERE " + strings.Join(notNull, " AND ") + " AND NOT EXISTS (SELECT 1 FROM " + quoteID(rels[0].Table) + " p WHERE " + strings.Join(conditions, " AND ") + ")"
			var count int64
			if e := tx.QueryRowContext(ctx, query).Scan(&count); e != nil {
				return nil, e
			}
			if count > 0 {
				result = append(result, LegacyOrphan{Table: t.Name, Constraint: name, Columns: columns, Target: rels[0].Table, Count: count})
			}
		}
	}
	return result, nil
}

func legacyDigest(rows [][]*string) string {
	b, _ := json.Marshal(rows)
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}
func readLegacyRows(ctx context.Context, q legacyQueryer, table LegacyTable, name string) ([][]*string, error) {
	columns := []string{}
	for _, c := range table.Columns {
		columns = append(columns, "HEX(CAST("+quoteID(c.Name)+" AS BINARY))")
	}
	rows, e := q.QueryContext(ctx, "SELECT "+strings.Join(columns, ",")+" FROM "+quoteID(name))
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	data := [][]*string{}
	for rows.Next() {
		values := make([]*string, len(columns))
		dest := make([]any, len(columns))
		for i := range dest {
			dest[i] = &values[i]
		}
		if e = rows.Scan(dest...); e != nil {
			return nil, e
		}
		data = append(data, values)
	}
	if e = rows.Err(); e != nil {
		return nil, e
	}
	sort.Slice(data, func(i, j int) bool {
		a, _ := json.Marshal(data[i])
		b, _ := json.Marshal(data[j])
		return string(a) < string(b)
	})
	return data, nil
}
func loadLegacySnapshot(ctx context.Context, db *sql.DB) (LegacySnapshot, error) {
	c, e := inspectLegacy(ctx, db)
	snapshot := LegacySnapshot{Catalog: c, Rows: map[string][][]*string{}}
	if e != nil {
		return snapshot, e
	}
	for _, t := range c.Tables {
		if t.Engine != "InnoDB" {
			return snapshot, fmt.Errorf("%s bukan InnoDB; snapshot konsisten tidak dapat dijamin", t.Name)
		}
	}
	tx, e := db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true, Isolation: sql.LevelRepeatableRead})
	if e != nil {
		return snapshot, e
	}
	defer tx.Rollback()
	for i := range snapshot.Catalog.Tables {
		t := &snapshot.Catalog.Tables[i]
		rows, e := readLegacyRows(ctx, tx, *t, t.Name)
		if e != nil {
			return snapshot, e
		}
		t.Count = int64(len(rows))
		t.Digest = legacyDigest(rows)
		snapshot.Rows[t.Name] = rows
	}
	snapshot.Catalog.Orphans, e = findLegacyOrphans(ctx, tx, snapshot.Catalog)
	if e != nil {
		return snapshot, e
	}
	if e = tx.Commit(); e != nil {
		return snapshot, e
	}
	return snapshot, validateLegacySnapshot(snapshot)
}

var ddlEnvelope = regexp.MustCompile("(?s)^CREATE TABLE `[A-Za-z_][A-Za-z0-9_]*` \\(\\n.*\\n\\) ENGINE=InnoDB(?: AUTO_INCREMENT=[0-9]+)? DEFAULT CHARSET=[A-Za-z0-9_]+(?: COLLATE=[A-Za-z0-9_]+)?$")
var referencePattern = regexp.MustCompile("(?i:REFERENCES)\\s+`([^`]+)`[ \\t]*\\(")
var referenceKeywordPattern = regexp.MustCompile(`(?i)\bREFERENCES\s+`)
var constraintPattern = regexp.MustCompile("CONSTRAINT `([^`]+)`")
var autoIncrementPattern = regexp.MustCompile(` AUTO_INCREMENT=\d+`)
var columnCharsetPattern = regexp.MustCompile(` CHARACTER SET ([A-Za-z0-9]+) COLLATE ([A-Za-z0-9_]+)`)

func normalizeLegacyDDL(ddl string) string {
	ddl = autoIncrementPattern.ReplaceAllString(ddl, "")
	ddl = columnCharsetPattern.ReplaceAllStringFunc(ddl, func(s string) string {
		m := columnCharsetPattern.FindStringSubmatch(s)
		if strings.HasPrefix(m[2], m[1]+"_") {
			return " COLLATE " + m[2]
		}
		return s
	})
	lines, constraints := []string{}, []string{}
	for _, line := range strings.Split(ddl, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "CONSTRAINT ") {
			constraints = append(constraints, strings.TrimSuffix(strings.TrimSpace(line), ","))
		} else {
			lines = append(lines, line)
		}
	}
	sort.Strings(constraints)
	return strings.Join(lines, "\n") + "\n" + strings.Join(constraints, "\n")
}

func mappedLegacyDDL(table LegacyTable, catalog LegacyCatalog) (string, error) {
	if !sqlIdentifier.MatchString(table.Name) || table.Target != "rw_"+table.Name || len(table.Target) > 64 {
		return "", errors.New("nama tabel salinan tidak valid")
	}
	ddl := table.DDL
	if !ddlEnvelope.MatchString(ddl) || !strings.HasPrefix(ddl, "CREATE TABLE "+quoteID(table.Name)+" (") || strings.ContainsAny(ddl, ";\x00") || strings.Contains(ddl, "/*") || strings.Contains(ddl, "--") {
		return "", fmt.Errorf("DDL %s tidak berada dalam format CREATE TABLE yang didukung", table.Name)
	}
	known := map[string]string{}
	for _, t := range catalog.Tables {
		known[t.Name] = t.Target
	}
	if len(referenceKeywordPattern.FindAllString(ddl, -1)) != len(referencePattern.FindAllString(ddl, -1)) {
		return "", errors.New("format referensi di luar snapshot tidak diizinkan")
	}
	for _, m := range referencePattern.FindAllStringSubmatch(ddl, -1) {
		if known[m[1]] == "" {
			return "", errors.New("referensi tabel di luar snapshot")
		}
	}
	ddl = strings.Replace(ddl, "CREATE TABLE "+quoteID(table.Name), "CREATE TABLE "+quoteID(table.Target), 1)
	ddl = referencePattern.ReplaceAllStringFunc(ddl, func(s string) string {
		m := referencePattern.FindStringSubmatch(s)
		return "REFERENCES " + quoteID(known[m[1]]) + " ("
	})
	ddl = constraintPattern.ReplaceAllStringFunc(ddl, func(s string) string {
		m := constraintPattern.FindStringSubmatch(s)
		sum := sha256.Sum256([]byte(table.Name + "/" + m[1]))
		return "CONSTRAINT " + quoteID("rw_"+hex.EncodeToString(sum[:12]))
	})
	return ddl, nil
}
func legacyOrder(c LegacyCatalog) ([]LegacyTable, error) {
	byName := map[string]LegacyTable{}
	for _, t := range c.Tables {
		byName[t.Name] = t
	}
	order := []LegacyTable{}
	visited := map[string]int{}
	var visit func(string) error
	visit = func(name string) error {
		if visited[name] == 2 {
			return nil
		}
		if visited[name] == 1 {
			return errors.New("siklus foreign key memerlukan migrasi khusus")
		}
		t, ok := byName[name]
		if !ok {
			return errors.New("referensi tabel tidak ditemukan")
		}
		visited[name] = 1
		for _, rel := range t.Relations {
			if rel.Table == name {
				return errors.New("foreign key ke tabel sendiri memerlukan migrasi khusus")
			}
			if e := visit(rel.Table); e != nil {
				return e
			}
		}
		visited[name] = 2
		order = append(order, t)
		return nil
	}
	for _, t := range c.Tables {
		if e := visit(t.Name); e != nil {
			return nil, e
		}
	}
	return order, nil
}
func validateLegacySnapshot(s LegacySnapshot) error {
	if len(s.Catalog.Tables) == 0 || len(s.Catalog.Tables) != len(s.Rows) {
		return errors.New("snapshot tabel tidak lengkap")
	}
	seen := map[string]bool{}
	for _, t := range s.Catalog.Tables {
		if seen[t.Name] {
			return errors.New("tabel ganda dalam snapshot")
		}
		seen[t.Name] = true
		if _, e := mappedLegacyDDL(t, s.Catalog); e != nil {
			return e
		}
		rows, ok := s.Rows[t.Name]
		if !ok || int64(len(rows)) != t.Count || legacyDigest(rows) != t.Digest {
			return fmt.Errorf("checksum/jumlah baris %s tidak cocok", t.Name)
		}
		for _, c := range t.Columns {
			if !sqlIdentifier.MatchString(c.Name) || strings.Contains(strings.ToUpper(c.Extra), "VIRTUAL GENERATED") || strings.Contains(strings.ToUpper(c.Extra), "STORED GENERATED") {
				return errors.New("kolom generated atau nama kolom tidak didukung")
			}
		}
		for _, row := range rows {
			if len(row) != len(t.Columns) {
				return errors.New("jumlah kolom snapshot tidak cocok")
			}
			for _, v := range row {
				if v != nil {
					if _, e := hex.DecodeString(*v); e != nil {
						return errors.New("encoding data snapshot tidak valid")
					}
				}
			}
		}
	}
	_, e := legacyOrder(s.Catalog)
	return e
}
func createLegacyTables(ctx context.Context, db *sql.DB, c LegacyCatalog) error {
	order, e := legacyOrder(c)
	if e != nil {
		return e
	}
	for _, t := range order {
		ddl, e := mappedLegacyDDL(t, c)
		if e != nil {
			return e
		}
		var exists int
		if e = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM information_schema.TABLES WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME=?", t.Target).Scan(&exists); e != nil {
			return e
		}
		if exists > 0 {
			var name, actual string
			if e = db.QueryRowContext(ctx, "SHOW CREATE TABLE "+quoteID(t.Target)).Scan(&name, &actual); e != nil {
				return e
			}
			if normalizeLegacyDDL(actual) != normalizeLegacyDDL(ddl) {
				return fmt.Errorf("struktur %s sudah ada dan berbeda", t.Target)
			}
			var count int
			if e = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+quoteID(t.Target)).Scan(&count); e != nil {
				return e
			}
			if count > 0 {
				return fmt.Errorf("%s sudah berisi data; tidak ditimpa", t.Target)
			}
			continue
		}
		if _, e = db.ExecContext(ctx, ddl); e != nil {
			return fmt.Errorf("membuat %s: %w", t.Target, e)
		}
	}
	return nil
}
func insertLegacySnapshot(ctx context.Context, tx *sql.Tx, s LegacySnapshot) error {
	// Existing broken links are historical data. Preserve them only in rw_ copies,
	// then restore FK checks before the transaction is committed.
	if len(s.Catalog.Orphans) > 0 {
		if _, e := tx.ExecContext(ctx, "SET SESSION FOREIGN_KEY_CHECKS=0"); e != nil {
			return e
		}
	}
	order, e := legacyOrder(s.Catalog)
	if e != nil {
		return e
	}
	for _, t := range order {
		columns := []string{}
		for _, c := range t.Columns {
			columns = append(columns, quoteID(c.Name))
		}
		query := "INSERT INTO " + quoteID(t.Target) + " (" + strings.Join(columns, ",") + ") VALUES (" + strings.TrimSuffix(strings.Repeat("?,", len(columns)), ",") + ")"
		stmt, e := tx.PrepareContext(ctx, query)
		if e != nil {
			return e
		}
		for _, row := range s.Rows[t.Name] {
			args := make([]any, len(row))
			for i, v := range row {
				if v != nil {
					b, _ := hex.DecodeString(*v)
					if strings.Contains(t.Columns[i].Type, "blob") || strings.Contains(t.Columns[i].Type, "binary") {
						args[i] = b
					} else {
						args[i] = string(b)
					}
				}
			}
			if _, e = stmt.ExecContext(ctx, args...); e != nil {
				stmt.Close()
				return fmt.Errorf("menyalin %s gagal: %w", t.Name, e)
			}
		}
		stmt.Close()
		copied, e := readLegacyRows(ctx, tx, t, t.Target)
		if e != nil {
			return e
		}
		if legacyDigest(copied) != t.Digest {
			return fmt.Errorf("verifikasi isi %s tidak cocok", t.Name)
		}
	}
	if _, e = tx.ExecContext(ctx, "SET SESSION FOREIGN_KEY_CHECKS=1"); e != nil {
		return e
	}
	b, e := json.Marshal(s.Catalog)
	if e != nil {
		return e
	}
	_, e = tx.ExecContext(ctx, "INSERT INTO registerweb_snapshot(id,catalog) VALUES(1,?)", string(b))
	return e
}
func (a *App) legacyCatalog() (LegacyCatalog, error) {
	var b []byte
	var c LegacyCatalog
	e := a.db.QueryRow("SELECT catalog FROM registerweb_snapshot WHERE id=1").Scan(&b)
	if e != nil {
		return c, e
	}
	e = json.Unmarshal(b, &c)
	return c, e
}
func (a *App) importRegisterWeb(ctx context.Context, source *sql.DB) (LegacyCatalog, error) {
	if c, e := a.legacyCatalog(); e == nil {
		return c, errors.New("salinan RegisterWeb sudah tersedia; impor tidak dijalankan ulang agar perubahan lokal tidak tertimpa")
	} else if e != sql.ErrNoRows {
		return c, e
	}
	s, e := loadLegacySnapshot(ctx, source)
	if e != nil {
		return s.Catalog, e
	}
	if e = createLegacyTables(ctx, a.db, s.Catalog); e != nil {
		return s.Catalog, e
	}
	tx, cleanup, e := beginArchiveTx(ctx, a.db)
	if e != nil {
		return s.Catalog, e
	}
	defer cleanup()
	if e = insertLegacySnapshot(ctx, tx, s); e != nil {
		return s.Catalog, e
	}
	if e = audit(tx, "system", "IMPORT_REGISTERWEB", fmt.Sprintf("%d tabel dari %s; checksum seluruh tabel cocok", len(s.Catalog.Tables), s.Catalog.Source)); e != nil {
		return s.Catalog, e
	}
	if e = tx.Commit(); e != nil {
		return s.Catalog, e
	}
	return s.Catalog, nil
}

type LegacyColumn struct {
	Name      string  `json:"name"`
	Type      string  `json:"type"`
	Nullable  bool    `json:"nullable"`
	Default   *string `json:"default"`
	Extra     string  `json:"extra"`
	Collation *string `json:"collation"`
}
type LegacyRelation struct {
	Name         string `json:"name"`
	Column       string `json:"column"`
	Table        string `json:"table"`
	TargetColumn string `json:"target_column"`
	Update       string `json:"on_update"`
	Delete       string `json:"on_delete"`
}
type LegacyIndex struct {
	Name     string `json:"name"`
	Column   string `json:"column"`
	Unique   bool   `json:"unique"`
	Position int    `json:"position"`
	Prefix   *int64 `json:"prefix"`
}
type LegacyTable struct {
	Name      string           `json:"name"`
	Target    string           `json:"target"`
	Engine    string           `json:"engine"`
	DDL       string           `json:"ddl"`
	Columns   []LegacyColumn   `json:"columns"`
	Relations []LegacyRelation `json:"relations"`
	Indexes   []LegacyIndex    `json:"indexes"`
	Count     int64            `json:"count"`
	Digest    string           `json:"sha256,omitempty"`
}
type LegacyCatalog struct {
	Scope   string         `json:"scope,omitempty"`
	Source  string         `json:"source"`
	Created string         `json:"created"`
	Tables  []LegacyTable  `json:"tables"`
	Orphans []LegacyOrphan `json:"orphans"`
}

var sqlIdentifier = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

func quoteID(name string) string { return "`" + strings.ReplaceAll(name, "`", "``") + "`" }

func inspectLegacy(ctx context.Context, db *sql.DB) (LegacyCatalog, error) {
	catalog := LegacyCatalog{Created: time.Now().UTC().Format(time.RFC3339), Tables: []LegacyTable{}}
	if e := db.QueryRowContext(ctx, "SELECT DATABASE()").Scan(&catalog.Source); e != nil {
		return catalog, e
	}
	rows, e := db.QueryContext(ctx, "SELECT TABLE_NAME,COALESCE(ENGINE,''),TABLE_TYPE FROM information_schema.TABLES WHERE TABLE_SCHEMA=DATABASE() ORDER BY TABLE_NAME")
	if e != nil {
		return catalog, e
	}
	for rows.Next() {
		var table LegacyTable
		var typ string
		if e = rows.Scan(&table.Name, &table.Engine, &typ); e != nil {
			rows.Close()
			return catalog, e
		}
		if typ != "BASE TABLE" {
			rows.Close()
			return catalog, fmt.Errorf("objek %s merupakan %s; perlu migrasi khusus", table.Name, typ)
		}
		if !sqlIdentifier.MatchString(table.Name) || len(table.Name) > 60 {
			rows.Close()
			return catalog, errors.New("nama tabel tidak dapat dipetakan secara aman")
		}
		table.Target = "rw_" + table.Name
		table.Columns = []LegacyColumn{}
		table.Relations = []LegacyRelation{}
		table.Indexes = []LegacyIndex{}
		catalog.Tables = append(catalog.Tables, table)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return catalog, e
	}
	var unsupported int
	if e = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM information_schema.TRIGGERS WHERE TRIGGER_SCHEMA=DATABASE()").Scan(&unsupported); e != nil {
		return catalog, e
	}
	if unsupported > 0 {
		return catalog, errors.New("sumber memiliki trigger; hentikan migrasi otomatis agar perilaku tidak hilang")
	}
	for i := range catalog.Tables {
		t := &catalog.Tables[i]
		var tableName string
		if e = db.QueryRowContext(ctx, "SHOW CREATE TABLE "+quoteID(t.Name)).Scan(&tableName, &t.DDL); e != nil {
			return catalog, e
		}
		if e = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+quoteID(t.Name)).Scan(&t.Count); e != nil {
			return catalog, e
		}
		rows, e = db.QueryContext(ctx, "SELECT COLUMN_NAME,COLUMN_TYPE,IS_NULLABLE,COLUMN_DEFAULT,EXTRA,COLLATION_NAME FROM information_schema.COLUMNS WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME=? ORDER BY ORDINAL_POSITION", t.Name)
		if e != nil {
			return catalog, e
		}
		for rows.Next() {
			var c LegacyColumn
			var nullable string
			if e = rows.Scan(&c.Name, &c.Type, &nullable, &c.Default, &c.Extra, &c.Collation); e != nil {
				rows.Close()
				return catalog, e
			}
			c.Nullable = nullable == "YES"
			t.Columns = append(t.Columns, c)
		}
		e = rows.Err()
		rows.Close()
		if e != nil {
			return catalog, e
		}
		rows, e = db.QueryContext(ctx, `SELECT k.CONSTRAINT_NAME,k.COLUMN_NAME,k.REFERENCED_TABLE_NAME,k.REFERENCED_COLUMN_NAME,r.UPDATE_RULE,r.DELETE_RULE FROM information_schema.KEY_COLUMN_USAGE k JOIN information_schema.REFERENTIAL_CONSTRAINTS r ON r.CONSTRAINT_SCHEMA=k.CONSTRAINT_SCHEMA AND r.TABLE_NAME=k.TABLE_NAME AND r.CONSTRAINT_NAME=k.CONSTRAINT_NAME WHERE k.TABLE_SCHEMA=DATABASE() AND k.TABLE_NAME=? AND k.REFERENCED_TABLE_NAME IS NOT NULL ORDER BY k.CONSTRAINT_NAME,k.ORDINAL_POSITION`, t.Name)
		if e != nil {
			return catalog, e
		}
		for rows.Next() {
			var rel LegacyRelation
			if e = rows.Scan(&rel.Name, &rel.Column, &rel.Table, &rel.TargetColumn, &rel.Update, &rel.Delete); e != nil {
				rows.Close()
				return catalog, e
			}
			t.Relations = append(t.Relations, rel)
		}
		e = rows.Err()
		rows.Close()
		if e != nil {
			return catalog, e
		}
		rows, e = db.QueryContext(ctx, "SELECT INDEX_NAME,COLUMN_NAME,NON_UNIQUE,SEQ_IN_INDEX,SUB_PART FROM information_schema.STATISTICS WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME=? ORDER BY INDEX_NAME,SEQ_IN_INDEX", t.Name)
		if e != nil {
			return catalog, e
		}
		for rows.Next() {
			var index LegacyIndex
			var nonUnique int
			if e = rows.Scan(&index.Name, &index.Column, &nonUnique, &index.Position, &index.Prefix); e != nil {
				rows.Close()
				return catalog, e
			}
			index.Unique = nonUnique == 0
			t.Indexes = append(t.Indexes, index)
		}
		e = rows.Err()
		rows.Close()
		if e != nil {
			return catalog, e
		}
	}
	return catalog, nil
}

func writeLegacyDocs(c LegacyCatalog) error {
	if e := os.MkdirAll("docs", 0755); e != nil {
		return e
	}
	b, e := json.MarshalIndent(c, "", "  ")
	if e != nil {
		return e
	}
	if e = os.WriteFile(filepath.Join("docs", "registerweb-schema.json"), b, 0600); e != nil {
		return e
	}
	var doc strings.Builder
	fmt.Fprintf(&doc, "# Database RegisterWeb di LSPFI DMS\n\nSumber: `%s`. Diperiksa: %s. Tabel salinan memakai awalan `rw_` dalam `lspfi_dms`. Struktur berdasarkan database aktual, bukan hanya model Prisma.\n\n", c.Source, c.Created)
	doc.WriteString("| Tabel sumber | Tabel salinan | Baris | Kolom | Relasi |\n|---|---|---:|---:|---:|\n")
	for _, t := range c.Tables {
		fmt.Fprintf(&doc, "| %s | %s | %d | %d | %d |\n", t.Name, t.Target, t.Count, len(t.Columns), len(t.Relations))
	}
	if len(c.Orphans) > 0 {
		doc.WriteString("\n## Relasi putus yang sudah ada di sumber\n\nSeluruh baris tetap disalin tanpa memperbaiki identitas secara otomatis. Pemeriksaan foreign key hanya dinonaktifkan pada koneksi penyalinan tabel rw_ selama pengisian data historis, kemudian diaktifkan kembali sebelum commit.\n\n| Tabel | Kolom | Tabel induk | Baris tanpa induk |\n|---|---|---|---:|\n")
		for _, o := range c.Orphans {
			fmt.Fprintf(&doc, "| %s | %s | %s | %d |\n", o.Table, strings.Join(o.Columns, ", "), o.Target, o.Count)
		}
	}
	for _, t := range c.Tables {
		fmt.Fprintf(&doc, "\n## %s\n\nEngine: %s. Baris: %d.\n\n| Kolom | Tipe | Nullable | Default | Tambahan |\n|---|---|---|---|---|\n", t.Name, t.Engine, t.Count)
		for _, col := range t.Columns {
			def := "NULL"
			if col.Default != nil {
				def = *col.Default
			}
			fmt.Fprintf(&doc, "| %s | %s | %t | %s | %s |\n", col.Name, strings.ReplaceAll(col.Type, "|", "\\|"), col.Nullable, strings.ReplaceAll(def, "|", "\\|"), col.Extra)
		}
		doc.WriteString("\nIndeks:\n\n")
		for _, idx := range t.Indexes {
			fmt.Fprintf(&doc, "- `%s`: `%s` (urutan %d, unik %t).\n", idx.Name, idx.Column, idx.Position, idx.Unique)
		}
		if len(t.Relations) > 0 {
			doc.WriteString("\nRelasi:\n\n")
			for _, rel := range t.Relations {
				fmt.Fprintf(&doc, "- `%s` → `%s.%s`; ON UPDATE %s; ON DELETE %s.\n", rel.Column, rel.Table, rel.TargetColumn, rel.Update, rel.Delete)
			}
		}
		if t.Digest != "" {
			fmt.Fprintf(&doc, "\nSHA-256 isi salinan: `%s`.\n", t.Digest)
		}
	}
	return os.WriteFile(filepath.Join("docs", "registerweb-database.md"), []byte(doc.String()), 0600)
}
