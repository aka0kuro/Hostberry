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
      const statApi = document.getElementById('blocky-stat-api');
      const statGroups = document.getElementById('blocky-stat-groups');

      if (!indicator || !statusText) return;

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
      if (statApi) {
        statApi.textContent = status?.active && status?.blocking_enabled !== undefined ? 'OK' : '—';
      }
      if (statGroups) {
        const groups = status?.disabled_groups;
        if (Array.isArray(groups) && groups.length > 0) {
          statGroups.textContent = groups.join(', ');
        } else {
          statGroups.textContent = status?.active ? (t('blocky.stat_none', 'None') || 'None') : '—';
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

  function showBlockyConfig() {
    const form = document.getElementById('blocky-config-form');
    if (!form) return;
    form.style.display = form.style.display === 'none' ? 'block' : 'none';
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
      const blockListsText = document.getElementById('blocky-block-lists')?.value || '';
      const upstreams = upstreamsText.split(/\n/).map(s => s.trim()).filter(Boolean);
      const block_lists = blockListsText.split(/\n/).map(s => s.trim()).filter(Boolean);

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
    bindForms();
    setInterval(loadBlockyStatus, 30000);
  });
})();
