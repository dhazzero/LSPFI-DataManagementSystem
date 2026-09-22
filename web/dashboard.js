/**
 * LSPFI Executive Dashboard Logic
 * Enterprise Dark Mode Analytics & Reactive Visualizations
 */

(function () {
  'use strict';

  // Application State
  const state = {
    allRecords: [],
    filteredRecords: [],
    masters: [],
    schemesMap: {},
    regSummary: null,
    charts: {
      timeline: null,
      schemes: null,
      companies: null
    },
    filter: {
      year: '',
      scheme: '',
      result: '',
      search: '',
      tabStatus: 'all'
    },
    pagination: {
      page: 1,
      pageSize: 12
    }
  };

  // Helper: Format Number with commas/dots
  function formatNumber(n) {
    if (n === null || n === undefined || isNaN(n)) return '0';
    return Number(n).toLocaleString('id-ID');
  }

  // Helper: Safe HTML Escaping
  function escapeHtml(str) {
    if (!str) return '';
    return String(str)
      .replace(/&/g, '&amp;')
      .replace(/</g, '&lt;')
      .replace(/>/g, '&gt;')
      .replace(/"/g, '&quot;')
      .replace(/'/g, '&#039;');
  }

  // Initialize and Fetch Backend Data
  async function init() {
    updateDateDisplay();
    bindEvents();
    await loadData();
  }

  function updateDateDisplay() {
    const now = new Date();
    const options = { weekday: 'long', year: 'numeric', month: 'short', day: 'numeric' };
    const dateStr = now.toLocaleDateString('id-ID', options);
    const dateEl = document.getElementById('header-date');
    if (dateEl) dateEl.textContent = dateStr;
  }

  async function loadData() {
    const syncEl = document.getElementById('header-sync');
    const refreshIcon = document.getElementById('refresh-icon');
    if (syncEl) syncEl.textContent = 'Sinkronisasi: Memuat data…';
    if (refreshIcon) refreshIcon.classList.add('animate-spin');

    try {
      // Parallel fetch from Go backend
      const [recordsRes, metaRes, regRes] = await Promise.all([
        fetch('/api/records').then(r => r.ok ? r.json() : Promise.reject(r)),
        fetch('/api/meta').then(r => r.ok ? r.json() : Promise.reject(r)),
        fetch('/api/registrations').then(r => r.ok ? r.json() : null).catch(() => null)
      ]);

      state.allRecords = recordsRes || [];
      state.masters = (metaRes && metaRes.masters) ? metaRes.masters : [];
      state.regSummary = regRes;

      // Build Scheme Code -> Label Dictionary
      state.schemesMap = {};
      state.masters.forEach(m => {
        if (m.category === 'SKEMA') {
          state.schemesMap[m.code] = m.label;
        }
      });

      // Populate Filter Options
      populateFilters();

      // Apply initial filters & render
      applyFilters();

      const timeNow = new Date().toLocaleTimeString('id-ID', { hour: '2-digit', minute: '2-digit', second: '2-digit' });
      if (syncEl) syncEl.textContent = `Sinkronisasi: ${timeNow} WIB`;
    } catch (err) {
      console.error('Gagal memuat data dashboard:', err);
      if (syncEl) syncEl.textContent = 'Gagal sinkronisasi data';
      
      // Check if unauthenticated
      if (err && (err.status === 401 || err.status === 403)) {
        renderAuthRequired();
      } else {
        renderLoadError(err);
      }
    } finally {
      if (refreshIcon) refreshIcon.classList.remove('animate-spin');
    }
  }

  function renderAuthRequired() {
    const tbody = document.getElementById('table-body');
    if (tbody) {
      tbody.innerHTML = `
        <tr>
          <td colspan="7" class="py-16 text-center text-slate-400">
            <div class="text-3xl mb-3">🔒</div>
            <div class="text-base font-semibold text-slate-200">Sesi Masuk Diperlukan</div>
            <p class="text-xs text-slate-400 max-w-md mx-auto mt-1 mb-5">
              Anda perlu masuk ke sistem LSPFI Arsip Digital terlebih dahulu untuk mengakses data analitik ini.
            </p>
            <a href="/" class="inline-flex items-center gap-2 px-4 py-2 rounded-lg bg-brand-600 hover:bg-brand-500 text-white text-xs font-semibold shadow-lg shadow-brand-950/50 transition-colors">
              Buka Halaman Masuk →
            </a>
          </td>
        </tr>
      `;
    }
  }

  function renderLoadError(err) {
    const tbody = document.getElementById('table-body');
    if (tbody) {
      tbody.innerHTML = `
        <tr>
          <td colspan="7" class="py-12 text-center text-rose-400">
            <div class="text-2xl mb-2">⚠</div>
            <div class="font-semibold text-sm">Gagal Mengambil Data Backend</div>
            <p class="text-xs text-slate-400 mt-1">${escapeHtml(err.message || 'Periksa koneksi database MySQL dan server lokal Go.')}</p>
          </td>
        </tr>
      `;
    }
  }

  // Populate dynamic dropdown filters based on active records
  function populateFilters() {
    const yearSelect = document.getElementById('filter-year');
    const schemeSelect = document.getElementById('filter-scheme');

    // Extract Years
    const yearsSet = new Set();
    state.allRecords.forEach(r => {
      const d = (r.fields && r.fields.test_date) || '';
      if (d.length >= 4) {
        const yr = d.substring(0, 4);
        if (/^\d{4}$/.test(yr)) yearsSet.add(yr);
      }
    });

    const sortedYears = Array.from(yearsSet).sort().reverse();
    if (yearSelect) {
      const currentYearVal = yearSelect.value;
      yearSelect.innerHTML = '<option value="">Semua Tahun</option>' +
        sortedYears.map(y => `<option value="${y}" ${y === currentYearVal ? 'selected' : ''}>Tahun ${y}</option>`).join('');
    }

    // Populate Schemes
    if (schemeSelect) {
      const currentSchemeVal = schemeSelect.value;
      const schemeCodes = Object.keys(state.schemesMap);
      
      // Also collect schemes present in records that might not be in master table
      state.allRecords.forEach(r => {
        const sc = (r.fields && r.fields.scheme) || '';
        if (sc && !schemeCodes.includes(sc)) {
          schemeCodes.push(sc);
        }
      });

      schemeSelect.innerHTML = '<option value="">Semua Skema Sertifikasi</option>' +
        schemeCodes.map(code => {
          const label = state.schemesMap[code] || `Skema #${code}`;
          return `<option value="${code}" ${code === currentSchemeVal ? 'selected' : ''}>${escapeHtml(label)}</option>`;
        }).join('');
    }
  }

  // Bind UI Events
  function bindEvents() {
    // Refresh button
    const btnRefresh = document.getElementById('btn-refresh');
    if (btnRefresh) {
      btnRefresh.addEventListener('click', () => loadData());
    }

    // Year filter
    const filterYear = document.getElementById('filter-year');
    if (filterYear) {
      filterYear.addEventListener('change', (e) => {
        state.filter.year = e.target.value;
        state.pagination.page = 1;
        applyFilters();
      });
    }

    // Scheme filter
    const filterScheme = document.getElementById('filter-scheme');
    if (filterScheme) {
      filterScheme.addEventListener('change', (e) => {
        state.filter.scheme = e.target.value;
        state.pagination.page = 1;
        applyFilters();
      });
    }

    // Result filter
    const filterResult = document.getElementById('filter-result');
    if (filterResult) {
      filterResult.addEventListener('change', (e) => {
        state.filter.result = e.target.value;
        state.pagination.page = 1;
        applyFilters();
      });
    }

    // Search input
    const searchInput = document.getElementById('table-search');
    if (searchInput) {
      let debounceTimer;
      searchInput.addEventListener('input', (e) => {
        clearTimeout(debounceTimer);
        debounceTimer = setTimeout(() => {
          state.filter.search = e.target.value.trim().toLowerCase();
          state.pagination.page = 1;
          applyFilters();
        }, 250);
      });
    }

    // Status Tab Buttons
    document.querySelectorAll('[data-tab-status]').forEach(btn => {
      btn.addEventListener('click', (e) => {
        document.querySelectorAll('[data-tab-status]').forEach(b => {
          b.className = 'px-2.5 py-1 rounded-md font-medium transition-colors text-slate-400 hover:text-white';
        });
        btn.className = 'px-2.5 py-1 rounded-md font-medium transition-colors bg-brand-600 text-white';
        state.filter.tabStatus = btn.getAttribute('data-tab-status');
        state.pagination.page = 1;
        applyFilters();
      });
    });

    // Pagination controls
    const btnPrev = document.getElementById('btn-prev-page');
    if (btnPrev) {
      btnPrev.addEventListener('click', () => {
        if (state.pagination.page > 1) {
          state.pagination.page--;
          renderTable();
        }
      });
    }

    const btnNext = document.getElementById('btn-next-page');
    if (btnNext) {
      btnNext.addEventListener('click', () => {
        const totalPages = Math.ceil(state.filteredRecords.length / state.pagination.pageSize) || 1;
        if (state.pagination.page < totalPages) {
          state.pagination.page++;
          renderTable();
        }
      });
    }

    // Export CSV
    const btnExport = document.getElementById('btn-export-csv');
    if (btnExport) {
      btnExport.addEventListener('click', exportToCsv);
    }
  }

  // Filter records according to active selections
  function applyFilters() {
    state.filteredRecords = state.allRecords.filter(r => {
      const f = r.fields || {};

      // Year filter
      if (state.filter.year) {
        const testYear = (f.test_date || '').substring(0, 4);
        if (testYear !== state.filter.year) return false;
      }

      // Scheme filter
      if (state.filter.scheme) {
        if (f.scheme !== state.filter.scheme) return false;
      }

      // Result filter (dropdown)
      if (state.filter.result) {
        if (state.filter.result === 'pending') {
          if (f.result === 'K' || f.result === 'BK') return false;
        } else if (f.result !== state.filter.result) {
          return false;
        }
      }

      // Tab Status filter
      if (state.filter.tabStatus !== 'all') {
        if (f.result !== state.filter.tabStatus) return false;
      }

      // Search query filter
      if (state.filter.search) {
        const q = state.filter.search;
        const name = (f.name || '').toLowerCase();
        const nik = (f.nik || '').toLowerCase();
        const comp = (f.company || '').toLowerCase();
        const reg = (f.registration || '').toLowerCase();
        const cert = (f.certificate || '').toLowerCase();
        const scName = (state.schemesMap[f.scheme] || '').toLowerCase();

        const match = name.includes(q) || nik.includes(q) || comp.includes(q) ||
                      reg.includes(q) || cert.includes(q) || scName.includes(q);
        if (!match) return false;
      }

      return true;
    });

    // Update UI components
    renderKpis();
    renderCharts();
    renderTable();
  }

  // Render 5 Top KPI Cards
  function renderKpis() {
    const records = state.filteredRecords;
    const total = records.length;

    let countK = 0;
    let countBK = 0;
    let countCert = 0;
    let countWithDocs = 0;
    const nikSet = new Set();
    const companySet = new Set();

    records.forEach(r => {
      const f = r.fields || {};
      const res = (f.result || '').toUpperCase();
      if (res === 'K') countK++;
      else if (res === 'BK') countBK++;

      if (f.certificate && f.certificate.trim() !== '') {
        countCert++;
      }

      if (f.nik && f.nik.trim() !== '') {
        nikSet.add(f.nik.trim());
      }

      if (f.company && f.company.trim() !== '' && f.company.trim() !== '-') {
        companySet.add(f.company.trim().toUpperCase());
      }

      if ((r.documents || 0) > 0) {
        countWithDocs++;
      }
    });

    const uniqueNiks = nikSet.size;
    const uniqueCompanies = companySet.size;

    // 1. Total Asesmen & Competency Rate
    const evaluatedTotal = countK + countBK;
    const passRate = evaluatedTotal > 0 ? ((countK / evaluatedTotal) * 100).toFixed(1) : (total > 0 && countK > 0 ? '100.0' : '0.0');
    
    const elTotalAssessments = document.getElementById('kpi-total-assessments');
    const elPassRate = document.getElementById('kpi-pass-rate');
    const elPassBar = document.getElementById('kpi-pass-bar');
    if (elTotalAssessments) elTotalAssessments.textContent = formatNumber(total);
    if (elPassRate) elPassRate.textContent = `${passRate}% (${formatNumber(countK)} K / ${formatNumber(countBK)} BK)`;
    if (elPassBar) elPassBar.style.width = `${Math.min(100, Math.max(0, passRate))}%`;

    // 2. Asesi Unik (NIK)
    const elUniqueCandidates = document.getElementById('kpi-unique-candidates');
    const elCandidateRatio = document.getElementById('kpi-candidate-ratio');
    if (elUniqueCandidates) elUniqueCandidates.textContent = formatNumber(uniqueNiks);
    if (elCandidateRatio) {
      const ratio = uniqueNiks > 0 ? (total / uniqueNiks).toFixed(2) : '1.00';
      elCandidateRatio.textContent = `Rata-rata ${ratio}x asesmen/kandidat`;
    }

    // 3. Cakupan Sertifikat Terbit
    const certRatio = countK > 0 ? ((countCert / countK) * 100).toFixed(1) : (countCert > 0 ? '100.0' : '0.0');
    const elCertCount = document.getElementById('kpi-cert-count');
    const elCertRatio = document.getElementById('kpi-cert-ratio');
    const elCertBar = document.getElementById('kpi-cert-bar');
    if (elCertCount) elCertCount.textContent = formatNumber(countCert);
    if (elCertRatio) elCertRatio.textContent = `${certRatio}% (${formatNumber(countCert)} dari ${formatNumber(countK)} lulus)`;
    if (elCertBar) elCertBar.style.width = `${Math.min(100, Math.max(0, certRatio))}%`;

    // 4. Kepatuhan Berkas Digital (Audit Readiness)
    const docCompliance = total > 0 ? ((countWithDocs / total) * 100).toFixed(1) : '0.0';
    const elDocCompliance = document.getElementById('kpi-doc-compliance');
    const elDocCount = document.getElementById('kpi-doc-count');
    const elDocBar = document.getElementById('kpi-doc-bar');
    if (elDocCompliance) elDocCompliance.textContent = `${docCompliance}%`;
    if (elDocCount) elDocCount.textContent = `${formatNumber(countWithDocs)} dari ${formatNumber(total)} terunggah`;
    if (elDocBar) elDocBar.style.width = `${Math.min(100, Math.max(0, docCompliance))}%`;

    // 5. Mitra Industri Fintech
    const elCompaniesCount = document.getElementById('kpi-companies-count');
    const elCorporateReach = document.getElementById('kpi-corporate-reach');
    if (elCompaniesCount) elCompaniesCount.textContent = formatNumber(uniqueCompanies);
    if (elCorporateReach) elCorporateReach.textContent = `${formatNumber(uniqueCompanies)} Entitas Perusahaan`;
  }

  // Render Interactive Charts (Chart.js)
  function renderCharts() {
    if (typeof Chart === 'undefined') {
      console.warn('Chart.js belum siap.');
      return;
    }

    renderTimelineChart();
    renderSchemesChart();
    renderCompaniesChart();
  }

  // Chart 1: Timeline Trend
  function renderTimelineChart() {
    const canvas = document.getElementById('chart-timeline');
    if (!canvas) return;

    // Group counts by Year-Month
    const monthlyData = {};
    state.filteredRecords.forEach(r => {
      const d = (r.fields && r.fields.test_date) || '';
      let period = 'Belum Dijadwalkan';
      if (d.length >= 7 && /^\d{4}-\d{2}/.test(d)) {
        period = d.substring(0, 7);
      } else if (d.length >= 4 && /^\d{4}/.test(d)) {
        period = d.substring(0, 4);
      }

      if (!monthlyData[period]) {
        monthlyData[period] = { total: 0, k: 0, bk: 0 };
      }
      monthlyData[period].total++;
      const res = (r.fields && r.fields.result || '').toUpperCase();
      if (res === 'K') monthlyData[period].k++;
      else if (res === 'BK') monthlyData[period].bk++;
    });

    // Sort periods
    const periods = Object.keys(monthlyData).sort();
    // Keep last 15 periods if too many
    const displayPeriods = periods.length > 15 ? periods.slice(-15) : periods;

    const labels = displayPeriods.map(p => {
      if (p.includes('-')) {
        const [yr, m] = p.split('-');
        const monthNames = ['', 'Jan', 'Feb', 'Mar', 'Apr', 'Mei', 'Jun', 'Jul', 'Agu', 'Sep', 'Okt', 'Nov', 'Des'];
        return `${monthNames[parseInt(m, 10)] || m} ${yr}`;
      }
      return p;
    });

    const datasetK = displayPeriods.map(p => monthlyData[p].k);
    const datasetBK = displayPeriods.map(p => monthlyData[p].bk);
    const datasetTotal = displayPeriods.map(p => monthlyData[p].total);

    if (state.charts.timeline) {
      state.charts.timeline.destroy();
    }

    const ctx = canvas.getContext('2d');
    const gradK = ctx.createLinearGradient(0, 0, 0, 260);
    gradK.addColorStop(0, 'rgba(20, 184, 166, 0.4)');
    gradK.addColorStop(1, 'rgba(20, 184, 166, 0.0)');

    state.charts.timeline = new Chart(canvas, {
      type: 'line',
      data: {
        labels: labels.length ? labels : ['Tidak Ada Data'],
        datasets: [
          {
            label: 'Kompeten (K)',
            data: datasetK.length ? datasetK : [0],
            borderColor: '#14b8a6',
            backgroundColor: gradK,
            borderWidth: 2.5,
            fill: true,
            tension: 0.35,
            pointRadius: 4,
            pointHoverRadius: 6,
            pointBackgroundColor: '#14b8a6'
          },
          {
            label: 'Belum Kompeten (BK)',
            data: datasetBK.length ? datasetBK : [0],
            borderColor: '#f43f5e',
            backgroundColor: 'rgba(244, 63, 94, 0.05)',
            borderWidth: 2,
            borderDash: [4, 4],
            fill: false,
            tension: 0.35,
            pointRadius: 3,
            pointHoverRadius: 5,
            pointBackgroundColor: '#f43f5e'
          },
          {
            label: 'Total Asesmen',
            data: datasetTotal.length ? datasetTotal : [0],
            borderColor: '#64748b',
            backgroundColor: 'transparent',
            borderWidth: 1.5,
            pointRadius: 2,
            tension: 0.35
          }
        ]
      },
      options: {
        responsive: true,
        maintainAspectRatio: false,
        interaction: {
          mode: 'index',
          intersect: false
        },
        plugins: {
          legend: {
            display: false
          },
          tooltip: {
            backgroundColor: '#0f172a',
            borderColor: '#334155',
            borderWidth: 1,
            titleColor: '#e2e8f0',
            bodyColor: '#cbd5e1',
            padding: 10,
            boxPadding: 4,
            usePointStyle: true
          }
        },
        scales: {
          x: {
            grid: {
              color: 'rgba(51, 65, 85, 0.3)'
            },
            ticks: {
              color: '#94a3b8',
              font: { size: 10 }
            }
          },
          y: {
            beginAtZero: true,
            grid: {
              color: 'rgba(51, 65, 85, 0.3)'
            },
            ticks: {
              color: '#94a3b8',
              font: { size: 10 },
              precision: 0
            }
          }
        }
      }
    });
  }

  // Chart 2: Scheme Distribution
  function renderSchemesChart() {
    const canvas = document.getElementById('chart-schemes');
    const noteEl = document.getElementById('chart-schemes-note');
    if (!canvas) return;

    const schemeCounts = {};
    state.filteredRecords.forEach(r => {
      const sc = (r.fields && r.fields.scheme) || 'N/A';
      schemeCounts[sc] = (schemeCounts[sc] || 0) + 1;
    });

    const entries = Object.entries(schemeCounts).sort((a, b) => b[1] - a[1]);
    const topEntries = entries.slice(0, 6);
    const otherCount = entries.slice(6).reduce((acc, curr) => acc + curr[1], 0);
    if (otherCount > 0) {
      topEntries.push(['Lainnya', otherCount]);
    }

    const labels = topEntries.map(([code]) => {
      if (code === 'Lainnya') return 'Skema Lainnya';
      const label = state.schemesMap[code] || `Skema #${code}`;
      return label.length > 26 ? label.substring(0, 24) + '…' : label;
    });

    const data = topEntries.map(([, count]) => count);

    const colors = [
      '#14b8a6', // Teal
      '#06b6d4', // Cyan
      '#8b5cf6', // Violet
      '#f59e0b', // Amber
      '#ec4899', // Pink
      '#3b82f6', // Blue
      '#64748b'  // Slate
    ];

    if (state.charts.schemes) {
      state.charts.schemes.destroy();
    }

    if (noteEl) {
      noteEl.textContent = entries.length > 0
        ? `Menampilkan ${topEntries.length} dari total ${entries.length} skema aktif`
        : 'Tidak ada data skema untuk filter saat ini';
    }

    state.charts.schemes = new Chart(canvas, {
      type: 'doughnut',
      data: {
        labels: labels.length ? labels : ['Tidak Ada Data'],
        datasets: [{
          data: data.length ? data : [1],
          backgroundColor: data.length ? colors.slice(0, data.length) : ['#334155'],
          borderColor: '#0f172a',
          borderWidth: 2,
          hoverOffset: 6
        }]
      },
      options: {
        responsive: true,
        maintainAspectRatio: false,
        cutout: '72%',
        plugins: {
          legend: {
            display: false
          },
          tooltip: {
            backgroundColor: '#0f172a',
            borderColor: '#334155',
            borderWidth: 1,
            titleColor: '#e2e8f0',
            bodyColor: '#cbd5e1',
            padding: 10,
            callbacks: {
              label: function (ctx) {
                const total = ctx.dataset.data.reduce((a, b) => a + b, 0);
                const val = ctx.raw || 0;
                const pct = total > 0 ? ((val / total) * 100).toFixed(1) : 0;
                return ` ${val} Asesi (${pct}%)`;
              }
            }
          }
        }
      }
    });
  }

  // Chart 3: Top Fintech Companies
  function renderCompaniesChart() {
    const canvas = document.getElementById('chart-companies');
    if (!canvas) return;

    const companyCounts = {};
    state.filteredRecords.forEach(r => {
      let comp = (r.fields && r.fields.company) || 'Mandiri / Perseorangan';
      comp = comp.trim();
      if (!comp || comp === '-') comp = 'Mandiri / Perseorangan';
      companyCounts[comp] = (companyCounts[comp] || 0) + 1;
    });

    const entries = Object.entries(companyCounts).sort((a, b) => b[1] - a[1]);
    const top10 = entries.slice(0, 10);

    const labels = top10.map(([name]) => name.length > 32 ? name.substring(0, 30) + '…' : name);
    const data = top10.map(([, count]) => count);

    if (state.charts.companies) {
      state.charts.companies.destroy();
    }

    state.charts.companies = new Chart(canvas, {
      type: 'bar',
      data: {
        labels: labels.length ? labels : ['Tidak Ada Data'],
        datasets: [{
          label: 'Jumlah Asesi',
          data: data.length ? data : [0],
          backgroundColor: '#0d9488',
          hoverBackgroundColor: '#14b8a6',
          borderRadius: 6,
          barPercentage: 0.65
        }]
      },
      options: {
        indexAxis: 'y',
        responsive: true,
        maintainAspectRatio: false,
        plugins: {
          legend: {
            display: false
          },
          tooltip: {
            backgroundColor: '#0f172a',
            borderColor: '#334155',
            borderWidth: 1,
            titleColor: '#e2e8f0',
            bodyColor: '#cbd5e1',
            padding: 10
          }
        },
        scales: {
          x: {
            beginAtZero: true,
            grid: {
              color: 'rgba(51, 65, 85, 0.3)'
            },
            ticks: {
              color: '#94a3b8',
              font: { size: 10 },
              precision: 0
            }
          },
          y: {
            grid: {
              display: false
            },
            ticks: {
              color: '#cbd5e1',
              font: { size: 10 }
            }
          }
        }
      }
    });
  }

  // Render Compact Executive Table
  function renderTable() {
    const tbody = document.getElementById('table-body');
    const badgeTotal = document.getElementById('table-total-badge');
    const paginationInfo = document.getElementById('table-pagination-info');
    const currentPageEl = document.getElementById('table-current-page');
    const btnPrev = document.getElementById('btn-prev-page');
    const btnNext = document.getElementById('btn-next-page');

    const records = state.filteredRecords;
    const total = records.length;

    if (badgeTotal) badgeTotal.textContent = `${formatNumber(total)} Data`;

    if (total === 0) {
      if (tbody) {
        tbody.innerHTML = `
          <tr>
            <td colspan="7" class="py-12 text-center text-slate-500">
              <div class="text-2xl mb-2">∅</div>
              <div class="font-medium text-slate-400">Tidak ada data yang cocok dengan kriteria filter</div>
              <p class="text-xs text-slate-500 mt-1">Coba sesuaikan kata kunci pencarian atau bersihkan filter dropdown.</p>
            </td>
          </tr>
        `;
      }
      if (paginationInfo) paginationInfo.textContent = 'Menampilkan 0 data';
      if (currentPageEl) currentPageEl.textContent = '1 / 1';
      if (btnPrev) btnPrev.disabled = true;
      if (btnNext) btnNext.disabled = true;
      return;
    }

    const { page, pageSize } = state.pagination;
    const totalPages = Math.ceil(total / pageSize) || 1;
    const validPage = Math.max(1, Math.min(page, totalPages));
    state.pagination.page = validPage;

    const startIndex = (validPage - 1) * pageSize;
    const endIndex = Math.min(startIndex + pageSize, total);
    const pagedRecords = records.slice(startIndex, endIndex);

    if (paginationInfo) {
      paginationInfo.textContent = `Menampilkan ${startIndex + 1}–${endIndex} dari ${formatNumber(total)} riwayat asesmen`;
    }
    if (currentPageEl) {
      currentPageEl.textContent = `${validPage} / ${totalPages}`;
    }
    if (btnPrev) btnPrev.disabled = validPage <= 1;
    if (btnNext) btnNext.disabled = validPage >= totalPages;

    if (!tbody) return;

    tbody.innerHTML = pagedRecords.map(r => {
      const f = r.fields || {};
      const res = (f.result || '').toUpperCase();

      // Result badge styling
      let statusBadge = '';
      if (res === 'K') {
        statusBadge = `
          <span class="inline-flex items-center gap-1.5 px-2.5 py-0.5 rounded-full text-[11px] font-semibold bg-emerald-950/80 text-emerald-400 border border-emerald-800/60">
            <span class="w-1.5 h-1.5 rounded-full bg-emerald-400"></span>
            Kompeten (K)
          </span>
        `;
      } else if (res === 'BK') {
        statusBadge = `
          <span class="inline-flex items-center gap-1.5 px-2.5 py-0.5 rounded-full text-[11px] font-semibold bg-rose-950/80 text-rose-400 border border-rose-800/60">
            <span class="w-1.5 h-1.5 rounded-full bg-rose-400"></span>
            Belum Kompeten (BK)
          </span>
        `;
      } else {
        statusBadge = `
          <span class="inline-flex items-center gap-1.5 px-2.5 py-0.5 rounded-full text-[11px] font-semibold bg-amber-950/80 text-amber-400 border border-amber-800/60">
            <span class="w-1.5 h-1.5 rounded-full bg-amber-400"></span>
            Dalam Proses
          </span>
        `;
      }

      // Document Scan badge
      const docCount = r.documents || 0;
      let docBadge = '';
      if (docCount > 0) {
        docBadge = `
          <span class="inline-flex items-center gap-1 px-2 py-0.5 rounded text-[10px] font-medium bg-teal-950/60 text-teal-300 border border-teal-800/50" title="${docCount} berkas scan terunggah">
            <span class="text-teal-400">✓</span> ${docCount} Berkas
          </span>
        `;
      } else {
        docBadge = `
          <span class="inline-flex items-center gap-1 px-2 py-0.5 rounded text-[10px] font-medium bg-slate-800/80 text-slate-500 border border-slate-700/50" title="Belum ada berkas scan">
            Tanpa Scan
          </span>
        `;
      }

      // Scheme Name
      const schemeLabel = state.schemesMap[f.scheme] || (f.scheme ? `Skema #${f.scheme}` : '-');

      // Identification numbers
      const regNo = f.registration || '-';
      const certNo = f.certificate || '-';

      return `
        <tr class="hover:bg-slate-900/40 transition-colors">
          <td class="py-3 px-4">
            <div class="font-semibold text-slate-100">${escapeHtml(f.name || 'Tanpa Nama')}</div>
            <div class="text-[10px] font-mono text-slate-400 mt-0.5">${escapeHtml(f.nik || '-')}</div>
          </td>
          <td class="py-3 px-4 max-w-[220px]">
            <div class="text-slate-200 truncate font-medium" title="${escapeHtml(schemeLabel)}">
              ${escapeHtml(schemeLabel)}
            </div>
            <div class="text-[10px] text-slate-500">ID: ${escapeHtml(f.scheme || '-')}</div>
          </td>
          <td class="py-3 px-4 max-w-[200px]">
            <div class="text-slate-200 truncate font-medium" title="${escapeHtml(f.company || '-')}">
              ${escapeHtml(f.company || 'Mandiri')}
            </div>
            <div class="text-[10px] text-slate-400 truncate">${escapeHtml(f.tuk || '-')}</div>
          </td>
          <td class="py-3 px-4 whitespace-nowrap text-slate-300 font-mono text-[11px]">
            ${escapeHtml(f.test_date || '-')}
          </td>
          <td class="py-3 px-4">
            <div class="text-[11px] font-mono text-slate-200 truncate max-w-[200px]" title="No. Reg: ${escapeHtml(regNo)}">
              <span class="text-slate-500">Reg:</span> ${escapeHtml(regNo)}
            </div>
            <div class="text-[10px] font-mono text-brand-400/90 truncate max-w-[200px]" title="No. Sertifikat: ${escapeHtml(certNo)}">
              <span class="text-slate-500">Cert:</span> ${escapeHtml(certNo)}
            </div>
          </td>
          <td class="py-3 px-4 text-center whitespace-nowrap">
            ${docBadge}
          </td>
          <td class="py-3 px-4 text-right whitespace-nowrap">
            ${statusBadge}
          </td>
        </tr>
      `;
    }).join('');
  }

  // Export filtered dataset to CSV
  function exportToCsv() {
    const records = state.filteredRecords;
    if (!records || records.length === 0) {
      alert('Tidak ada data untuk diekspor.');
      return;
    }

    const headers = [
      'ID',
      'NIK',
      'Nama Asesi',
      'Kode Skema',
      'Nama Skema',
      'Perusahaan',
      'TUK',
      'Tanggal Uji',
      'Nomor Registrasi',
      'Nomor Sertifikat',
      'Jumlah Dokumen',
      'Keputusan'
    ];

    const rows = records.map(r => {
      const f = r.fields || {};
      const scLabel = state.schemesMap[f.scheme] || f.scheme || '';
      return [
        r.id || '',
        `="${f.nik || ''}"`,
        `"${(f.name || '').replace(/"/g, '""')}"`,
        f.scheme || '',
        `"${scLabel.replace(/"/g, '""')}"`,
        `"${(f.company || '').replace(/"/g, '""')}"`,
        `"${(f.tuk || '').replace(/"/g, '""')}"`,
        f.test_date || '',
        `"${(f.registration || '').replace(/"/g, '""')}"`,
        `"${(f.certificate || '').replace(/"/g, '""')}"`,
        r.documents || 0,
        f.result || ''
      ].join(',');
    });

    const csvContent = '\uFEFF' + headers.join(',') + '\n' + rows.join('\n');
    const blob = new Blob([csvContent], { type: 'text/csv;charset=utf-8;' });
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = `LSPFI-Executive-Export-${new Date().toISOString().substring(0, 10)}.csv`;
    document.body.appendChild(a);
    a.click();
    document.body.removeChild(a);
    URL.revokeObjectURL(url);
  }

  // Self Init when DOM is loaded
  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', init);
  } else {
    init();
  }
})();
