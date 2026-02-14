// Adblock page: solo Blocky y su configuración
(function () {
  const HB = window.HostBerry || {};
  const t = HB.t || window.t || ((key, fallback) => fallback || key);
  const api = HB.apiRequest ? HB.apiRequest.bind(HB) : (url, opts) => {
    const o = Object.assign({ method: 'GET', headers: {} }, opts || {});
    const token = localStorage.getItem('access_token');
    const headers = new Headers(o.headers || {});
    if (token && !headers.has('Authorization')) headers.set('Authorization', 'Bearer ' + token);
    if (!headers.has('Content-Type')) headers.set('Content-Type', 'application/json');
    o.headers = headers;
    if (o.body && typeof o.body === 'object' && !(o.body instanceof FormData)) o.body = JSON.stringify(o.body);
    return fetch(url, o);
  };

  function notify(type, message) {
    if (HB.showAlert) return HB.showAlert(type, message);
    if (window.showAlert) return window.showAlert(type, message);
    alert(message);
  }

  async function readJson(resp) {
    return resp?.json?.().catch(() => ({}));
  }

  // --- Blocky ---
  async function loadBlockyStatus() {
    try {
      const resp = await api('/api/v1/adblock/blocky/status', { method: 'GET' });
      if (!resp || !resp.ok) return;
      const status = await readJson(resp);

      const indicator = document.getElementById('blocky-status-indicator');
      const statusText = document.getElementById('blocky-status-text');
      const installBtn = document.getElementById('blocky-install-btn');
      const configureBtn = document.getElementById('blocky-configure-btn');
      const enableBtn = document.getElementById('blocky-enable-btn');
      const disableBtn = document.getElementById('blocky-disable-btn');
      const refreshBtn = document.getElementById('blocky-refresh-lists-btn');
      const blockingRow = document.getElementById('blocky-blocking-row');
      const blockingValue = document.getElementById('blocky-blocking-value');
      const statService = document.getElementById('blocky-stat-service');
      const statBlocking = document.getElementById('blocky-stat-blocking');
      const statBlocked = document.getElementById('blocky-stat-blocked');
      const statTotal = document.getElementById('blocky-stat-total');
      const statCached = document.getElementById('blocky-stat-cached');
      const statApi = document.getElementById('blocky-stat-api');
      const statGroups = document.getElementById('blocky-stat-groups');

      if (!indicator || !statusText) return;

      function fmtNum(n) {
        if (n === undefined || n === null) return '—';
        const v = Number(n);
        if (isNaN(v)) return '—';
        return v.toLocaleString();
      }

      if (statService) {
        statService.textContent = status?.installed
          ? (status?.active ? t('blocky.active', 'Active') : t('blocky.inactive', 'Inactive'))
          : t('blocky.not_installed', 'Not installed');
      }
      if (statBlocking) {
        statBlocking.textContent = status?.active && status?.blocking_enabled !== undefined
          ? (status.blocking_enabled ? t('adblock.adblock_enabled', 'Enabled') : t('adblock.adblock_disabled', 'Disabled'))
          : '—';
      }
      if (statBlocked) {
        statBlocked.textContent = status?.active ? fmtNum(status.blocked_queries) : '—';
      }
      if (statTotal) {
        statTotal.textContent = status?.active ? fmtNum(status.total_queries) : '—';
      }
      if (statCached) {
        statCached.textContent = status?.active ? fmtNum(status.cached_queries) : '—';
      }
      if (statApi) {
        statApi.textContent = status?.active && status?.blocking_enabled !== undefined ? 'OK' : '—';
      }
      if (statGroups) {
        const groups = status?.disabled_groups;
        if (Array.isArray(groups) && groups.length > 0) {
          statGroups.textContent = groups.join(', ');
          statGroups.setAttribute('title', groups.join(', '));
        } else {
          statGroups.textContent = status?.active ? (t('blocky.stat_none', 'None') || 'None') : '—';
          statGroups.removeAttribute('title');
        }
      }

      if (status?.installed) {
        if (installBtn) installBtn.style.display = 'none';
        if (configureBtn) configureBtn.style.display = 'inline-block';

        if (status?.active) {
          indicator.className = 'status-indicator status-online';
          statusText.textContent = t('blocky.active', 'Active');
          if (enableBtn) enableBtn.style.display = 'none';
          if (disableBtn) disableBtn.style.display = 'inline-block';
          if (refreshBtn) refreshBtn.style.display = 'inline-block';
          if (blockingRow) blockingRow.style.display = 'block';
          if (blockingValue) {
            blockingValue.textContent = status?.blocking_enabled === true
              ? t('adblock.adblock_enabled', 'Enabled')
              : t('adblock.adblock_disabled', 'Disabled');
          }
        } else {
          indicator.className = 'status-indicator status-offline';
          statusText.textContent = t('blocky.inactive', 'Inactive');
          if (enableBtn) enableBtn.style.display = 'inline-block';
          if (disableBtn) disableBtn.style.display = 'none';
          if (refreshBtn) refreshBtn.style.display = 'none';
          if (blockingRow) blockingRow.style.display = 'none';
        }
      } else {
        indicator.className = 'status-indicator status-offline';
        statusText.textContent = t('blocky.not_installed', 'Not installed');
        if (installBtn) installBtn.style.display = 'inline-block';
        if (configureBtn) configureBtn.style.display = 'none';
        if (enableBtn) enableBtn.style.display = 'none';
        if (disableBtn) disableBtn.style.display = 'none';
        if (refreshBtn) refreshBtn.style.display = 'none';
        if (blockingRow) blockingRow.style.display = 'none';
      }
    } catch (error) {
      console.error('Error loading Blocky status:', error);
    }
  }

  async function installBlocky() {
    if (!confirm(t('blocky.install_confirm', 'Install Blocky? This will download the binary and create the systemd service.'))) return;
    try {
      const resp = await api('/api/v1/adblock/blocky/install', { method: 'POST' });
      const result = await readJson(resp);
      if (resp && resp.ok && result?.success !== false) {
        notify('success', result?.message || t('blocky.installed', 'Blocky installed'));
        loadBlockyStatus();
      } else {
        notify('danger', result?.error || t('errors.operation_failed', 'Operation failed'));
      }
    } catch (error) {
      console.error('Error installing Blocky:', error);
      notify('danger', t('errors.network_error', 'Network error. Please try again.'));
    }
  }

  // Listas sugeridas para Blocky: url, clave de nombre, clave de descripción
  const PRESET_LISTS = [
    { url: 'https://raw.githubusercontent.com/StevenBlack/hosts/master/hosts', nameKey: 'blocky.list_stevenblack_name', descKey: 'blocky.list_stevenblack_desc' },
    { url: 'https://adguardteam.github.io/AdGuardSDNSFilter/Filters/filter.txt', nameKey: 'blocky.list_adguard_ads_name', descKey: 'blocky.list_adguard_ads_desc' },
    { url: 'https://adguardteam.github.io/AdGuardSDNSFilter/Filters/tracking_filter.txt', nameKey: 'blocky.list_adguard_tracking_name', descKey: 'blocky.list_adguard_tracking_desc' },
    { url: 'https://adguardteam.github.io/AdGuardSDNSFilter/Filters/social_filter.txt', nameKey: 'blocky.list_adguard_social_name', descKey: 'blocky.list_adguard_social_desc' },
    { url: 'https://phishing.army/download/phishing_army_blocklist_extended.txt', nameKey: 'blocky.list_phishing_army_name', descKey: 'blocky.list_phishing_army_desc' },
    { url: 'https://raw.githubusercontent.com/hagezi/dns-blocklists/main/hosts/pro.txt', nameKey: 'blocky.list_hagezi_name', descKey: 'blocky.list_hagezi_desc' },
  ];

  function buildPresetListsUI() {
    const container = document.getElementById('blocky-preset-lists');
    if (!container) return;
    container.innerHTML = '';
    PRESET_LISTS.forEach(function (preset) {
      const id = 'preset-' + preset.url.replace(/[^a-zA-Z0-9]/g, '_').slice(0, 40);
      const label = document.createElement('label');
      label.className = 'd-block mb-2';
      label.setAttribute('for', id);
      const cb = document.createElement('input');
      cb.type = 'checkbox';
      cb.className = 'form-check-input me-2';
      cb.id = id;
      cb.value = preset.url;
      cb.dataset.url = preset.url;
      const strong = document.createElement('strong');
      strong.className = 'small';
      strong.textContent = t(preset.nameKey, preset.url);
      const desc = document.createElement('span');
      desc.className = 'd-block small text-muted mt-0 mb-1';
      desc.textContent = t(preset.descKey, '');
      label.appendChild(cb);
      label.appendChild(strong);
      label.appendChild(document.createElement('br'));
      label.appendChild(desc);
      container.appendChild(label);
    });
  }

  function getSelectedPresetUrls() {
    const container = document.getElementById('blocky-preset-lists');
    if (!container) return [];
    const urls = [];
    container.querySelectorAll('input[type="checkbox"]:checked').forEach(function (cb) {
      if (cb.value) urls.push(cb.value.trim());
    });
    return urls;
  }

  function setPresetCheckboxesFromUrls(urls) {
    const set = new Set((urls || []).map(function (u) { return u.trim(); }));
    const container = document.getElementById('blocky-preset-lists');
    if (!container) return;
    container.querySelectorAll('input[type="checkbox"]').forEach(function (cb) {
      cb.checked = set.has(cb.value.trim());
    });
  }

  async function loadBlockyConfigIntoForm() {
    try {
      const resp = await api('/api/v1/adblock/blocky/config', { method: 'GET' });
      if (!resp || !resp.ok) return;
      const cfg = await readJson(resp);
      const upstreams = cfg.upstreams || [];
      const blockLists = cfg.block_lists || [];
      const presetUrls = new Set(PRESET_LISTS.map(function (p) { return p.url; }));
      const customUrls = (blockLists || []).filter(function (u) {
        return !presetUrls.has(u.trim());
      });
      const upstreamEl = document.getElementById('blocky-upstreams');
      const listsEl = document.getElementById('blocky-block-lists');
      if (upstreamEl) upstreamEl.value = (upstreams.length ? upstreams.join('\n') : '1.1.1.1\n8.8.8.8\nhttps://dns.cloudflare.com/dns-query').trim();
      if (listsEl) listsEl.value = customUrls.join('\n').trim();
      setPresetCheckboxesFromUrls(blockLists);
    } catch (e) {
      console.error('Error loading Blocky config:', e);
    }
  }

  function showBlockyConfig() {
    const form = document.getElementById('blocky-config-form');
    if (!form) return;
    const opening = form.style.display === 'none';
    form.style.display = opening ? 'block' : 'none';
    if (opening) loadBlockyConfigIntoForm();
  }

  async function enableBlocky() {
    try {
      const resp = await api('/api/v1/adblock/blocky/enable', { method: 'POST' });
      const result = await readJson(resp);
      if (resp && resp.ok && result?.success !== false) {
        notify('success', result?.message || t('blocky.enabled', 'Enabled'));
        loadBlockyStatus();
        setTimeout(() => window.location.reload(), 1000);
      } else {
        notify('danger', result?.error || t('errors.operation_failed', 'Operation failed'));
      }
    } catch (error) {
      console.error('Error enabling Blocky:', error);
      notify('danger', t('errors.network_error', 'Network error. Please try again.'));
    }
  }

  async function disableBlocky() {
    if (!confirm(t('blocky.disable_confirm', 'Disable Blocky? DNS will fall back to system default.'))) return;
    try {
      const resp = await api('/api/v1/adblock/blocky/disable', { method: 'POST' });
      const result = await readJson(resp);
      if (resp && resp.ok && result?.success !== false) {
        notify('success', result?.message || t('blocky.disabled', 'Disabled'));
        loadBlockyStatus();
        setTimeout(() => window.location.reload(), 800);
      } else {
        notify('danger', result?.error || t('errors.operation_failed', 'Operation failed'));
      }
    } catch (error) {
      console.error('Error disabling Blocky:', error);
      notify('danger', t('errors.network_error', 'Network error. Please try again.'));
    }
  }

  async function blockyRefreshLists() {
    try {
      const resp = await api('/api/v1/adblock/blocky/api/lists/refresh', { method: 'POST' });
      if (resp && resp.ok) {
        notify('success', t('blocky.lists_refreshed', 'Blocky lists refreshed'));
        loadBlockyStatus();
      } else {
        notify('danger', t('blocky.refresh_failed', 'Blocky did not respond. Is the service running?'));
      }
    } catch (error) {
      notify('danger', t('errors.network_error', 'Network error. Please try again.'));
    }
  }

  function bindForms() {
    document.getElementById('blockyConfigForm')?.addEventListener('submit', async function (e) {
      e.preventDefault();
      const upstreamsText = document.getElementById('blocky-upstreams')?.value || '';
      const upstreams = upstreamsText.split(/\n/).map(s => s.trim()).filter(Boolean);
      const selectedPresets = getSelectedPresetUrls();
      const customText = document.getElementById('blocky-block-lists')?.value || '';
      const customUrls = customText.split(/\n/).map(s => s.trim()).filter(Boolean);
      const block_lists = selectedPresets.concat(customUrls);

      try {
        const resp = await api('/api/v1/adblock/blocky/configure', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ upstreams, block_lists }),
        });
        const result = await readJson(resp);
        if (resp && resp.ok && result?.success !== false) {
          notify('success', result?.message || t('blocky.configured', 'Blocky configured'));
          document.getElementById('blocky-config-form').style.display = 'none';
        } else {
          notify('danger', result?.error || t('errors.operation_failed', 'Operation failed'));
        }
      } catch (error) {
        console.error('Error configuring Blocky:', error);
        notify('danger', t('errors.network_error', 'Network error. Please try again.'));
      }
    });
  }

  window.loadBlockyStatus = loadBlockyStatus;
  window.installBlocky = installBlocky;
  window.showBlockyConfig = showBlockyConfig;
  window.enableBlocky = enableBlocky;
  window.disableBlocky = disableBlocky;
  window.blockyRefreshLists = blockyRefreshLists;

  document.addEventListener('DOMContentLoaded', () => {
    loadBlockyStatus();
    buildPresetListsUI();
    bindForms();
    setInterval(loadBlockyStatus, 30000);
  });
})();
