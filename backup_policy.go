package main

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sort"
	"strings"
)

const referenceBackupScope = "references-without-asesi"

// Explicit inclusion keeps new/unknown operational tables out of backups.
func referenceLegacyTable(name string) bool {
	switch name {
	case "skema", "unitkompetensi", "elemenkompetensi", "kriteriaunjukkerja",
		"banksoal", "paketsoal", "paketsoalitem", "parameterbnsp",
		"certificatesequence", "registrationsequence", "user":
		return true
	}
	return false
}

func legacyCell(t LegacyTable, row []*string, column string) string {
	for i, c := range t.Columns {
		if c.Name == column && i < len(row) && row[i] != nil {
			b, _ := hex.DecodeString(*row[i])
			return string(b)
		}
	}
	return ""
}

func referenceLegacySnapshot(source LegacySnapshot) LegacySnapshot {
	result := LegacySnapshot{Catalog: source.Catalog, Rows: map[string][][]*string{}}
	result.Catalog.Tables = append([]LegacyTable(nil), source.Catalog.Tables...)
	result.Catalog.Orphans = []LegacyOrphan{}
	result.Catalog.Scope = referenceBackupScope
	// A changed role must not accidentally include an account with an asesi profile.
	asesiIDs, staffIDs := map[string]bool{}, map[string]bool{}
	for _, t := range source.Catalog.Tables {
		if t.Name == "asesiprofile" || t.Name == "pendaftaran" {
			for _, row := range source.Rows[t.Name] {
				asesiIDs[legacyCell(t, row, "user_id")] = true
			}
		}
	}
	for _, t := range source.Catalog.Tables {
		if t.Name != "user" {
			continue
		}
		for _, row := range source.Rows[t.Name] {
			id := legacyCell(t, row, "id")
			role := strings.ToLower(strings.TrimSpace(legacyCell(t, row, "role")))
			if id != "" && !asesiIDs[id] && (role == "superadmin" || role == "admin" || role == "asesor") {
				staffIDs[id] = true
			}
		}
	}
	for i := range result.Catalog.Tables {
		t := &result.Catalog.Tables[i]
		rows := [][]*string{}
		if referenceLegacyTable(t.Name) {
			for _, row := range source.Rows[t.Name] {
				if t.Name == "user" && !staffIDs[legacyCell(*t, row, "id")] {
					continue
				}
				copyRow := append([]*string(nil), row...)
				if t.Name == "banksoal" && !staffIDs[legacyCell(*t, row, "created_by_user_id")] {
					for j, c := range t.Columns {
						if c.Name == "created_by_user_id" {
							copyRow[j] = nil
						}
					}
				}
				rows = append(rows, copyRow)
			}
		}
		sort.Slice(rows, func(i, j int) bool {
			a, _ := json.Marshal(rows[i])
			b, _ := json.Marshal(rows[j])
			return string(a) < string(b)
		})
		t.Count, t.Digest = int64(len(rows)), legacyDigest(rows)
		result.Rows[t.Name] = rows
	}
	for _, orphan := range source.Catalog.Orphans {
		if referenceLegacyTable(orphan.Table) && orphan.Table != "user" {
			result.Catalog.Orphans = append(result.Catalog.Orphans, orphan)
		}
	}
	return result
}

func validateReferenceBackup(b Backup) error {
	if b.Scope != referenceBackupScope || len(b.Files) != 0 {
		return errors.New("cadangan referensi tidak boleh berisi berkas asesi")
	}
	for name, rows := range b.Tables {
		if name != "users" && name != "master" && name != "registration_sequences" && len(rows) != 0 {
			return errors.New("cadangan referensi memuat data operasional asesi")
		}
	}
	if b.RegisterWeb != nil {
		if e := validateLegacySnapshot(*b.RegisterWeb); e != nil {
			return e
		}
		clean := referenceLegacySnapshot(*b.RegisterWeb)
		actual, _ := json.Marshal(b.RegisterWeb)
		want, _ := json.Marshal(clean)
		if !bytes.Equal(actual, want) {
			return errors.New("cadangan referensi RegisterWeb memuat data yang dikecualikan")
		}
	}
	return nil
}
