// AdBlock page logic (migrado desde script inline)
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

  function escapeHtml(value) {
    const s = String(value ?? '');
    return HB.escapeHtml
      ? HB.escapeHtml(s)
      : s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;');
  }

  // --- Lists (si el backend lo soporta) ---
  async function loadLists() {
    const tbody = document.getElementById('listsTable');
    if (!tbody) return;

    tbody.innerHTML = `
      <tr>
        <td colspan="5" class="text-center text-white-50 small py-3">
          <div class="spinner-border spinner-border-sm me-2" role="status"></div>
          ${t('common.loading', 'Loading...')}
        </td>
      </tr>`;

    try {
      const resp = await api('/api/v1/adblock/lists', { method: 'GET' });
      if (!resp || !resp.ok) {
        tbody.innerHTML = `
          <tr>
            <td colspan="5" class="text-center text-white-50 small py-3">
              ${t('adblock.lists_not_available', 'Lists not available in this build.')}
            </td>
          </tr>`;
        return;
      }

      const lists = await resp.json().catch(() => []);
      tbody.innerHTML = '';

      (Array.isArray(lists) ? lists : []).forEach((list) => {
        const enabled = !!list?.enabled;
        const listName = String(list?.name || '');
        const row = document.createElement('tr');
        row.innerHTML = `
          <td>${escapeHtml(listName)}</td>
          <td>
            <span class="badge bg-${enabled ? 'success' : 'danger'}">
              ${enabled ? t('common.enabled', 'Enabled') : t('common.disabled', 'Disabled')}
            </span>
          </td>
          <td>${escapeHtml(list?.domains_count ?? '')}</td>
          <td>${escapeHtml(list?.last_update ?? '')}</td>
          <td>
            <button class="btn btn-sm btn-outline-primary" type="button">
              <i class="bi bi-${enabled ? 'pause' : 'play'}"></i>
            </button>
          </td>
        `;
        const toggleBtn = row.querySelector('button');
        if (toggleBtn) toggleBtn.addEventListener('click', () => toggleList(listName));
        tbody.appendChild(row);
      });
    } catch (error) {
      console.error('Error loading lists:', error);
      tbody.innerHTML = `
        <tr>
          <td colspan="5" class="text-center text-white-50 small py-3">
            ${t('errors.network_error', 'Network error. Please try again.')}
          </td>
        </tr>`;
    }
  }

  // --- Domains (si el backend lo soporta) ---
  async function loadDomains() {
    const tbody = document.getElementById('domainsTable');
    if (!tbody) return;

    tbody.innerHTML = `
      <tr>
        <td colspan="3" class="text-center text-white-50 small py-3">
          <div class="spinner-border spinner-border-sm me-2" role="status"></div>
          ${t('common.loading', 'Loading...')}
        </td>
      </tr>`;

    try {
      const resp = await api('/api/v1/adblock/domains', { method: 'GET' });
      if (!resp || !resp.ok) {
        tbody.innerHTML = `
          <tr>
            <td colspan="3" class="text-center text-white-50 small py-3">
              ${t('adblock.domains_not_available', 'Domains list not available in this build.')}
            </td>
          </tr>`;
        return;
      }

      const domains = await resp.json().catch(() => []);
      tbody.innerHTML = '';

      (Array.isArray(domains) ? domains : []).slice(0, 10).forEach((domain) => {
        const blocked = !!domain?.blocked;
        const name = String(domain?.name || '');
        const row = document.createElement('tr');
        row.innerHTML = `
          <td>${escapeHtml(name)}</td>
          <td>
            <span class="badge bg-${blocked ? 'danger' : 'success'}">
              ${blocked ? t('adblock.blocked', 'Blocked') : t('adblock.allowed', 'Allowed')}
            </span>
          </td>
          <td>
            <button class="btn btn-sm btn-outline-${blocked ? 'success' : 'danger'}" type="button">
              <i class="bi bi-${blocked ? 'check' : 'x'}"></i>
            </button>
          </td>
        `;
        const toggleBtn = row.querySelector('button');
        if (toggleBtn) toggleBtn.addEventListener('click', () => toggleDomain(name));
        tbody.appendChild(row);
      });
    } catch (error) {
      console.error('Error loading domains:', error);
      tbody.innerHTML = `
        <tr>
          <td colspan="3" class="text-center text-white-50 small py-3">
            ${t('errors.network_error', 'Network error. Please try again.')}
          </td>
        </tr>`;
    }
  }

  // --- Core actions ---
  async function toggleAdBlock() {
    try {
      // Preferir endpoints reales (/enable /disable). Si no existen, caer al antiguo /toggle.
      const statusResp = await api('/api/v1/adblock/status', { method: 'GET' });
      const status = await readJson(statusResp);
      const isActive = !!(status?.active || status?.enabled);

      let endpoint = isActive ? '/api/v1/adblock/disable' : '/api/v1/adblock/enable';
      let resp = await api(endpoint, { method: 'POST' });

      // Compat: si no existe enable/disable, intentar /toggle
      if (resp && resp.status === 404) {
        endpoint = '/api/v1/adblock/toggle';
        resp = await api(endpoint, { method: 'POST' });
      }

      const payload = await readJson(resp);
      if (resp && resp.ok && payload?.success !== false) {
        notify('success', payload?.message || t('messages.operation_successful', 'Operation successful'));
        setTimeout(() => window.location.reload(), 800);
      } else {
        notify('danger', payload?.error || t('errors.operation_failed', 'Operation failed'));
      }
    } catch (error) {
      console.error('toggleAdBlock error:', error);
      notify('danger', t('errors.network_error', 'Network error. Please try again.'));
    }
  }

  async function updateLists() {
    try {
      const resp = await api('/api/v1/adblock/update', { method: 'POST' });
      const payload = await readJson(resp);
      if (resp && resp.ok && payload?.success !== false) {
        notify('info', payload?.message || t('adblock.updating_lists', 'Updating lists...'));
        setTimeout(loadLists, 2500);
      } else {
        notify('warning', payload?.error || t('errors.not_implemented', 'Not implemented'));
      }
    } catch (error) {
      console.error('updateLists error:', error);
      notify('danger', t('errors.network_error', 'Network error. Please try again.'));
    }
  }

  async function toggleList(listName) {
    try {
      const resp = await api(`/api/v1/adblock/lists/${encodeURIComponent(listName)}/toggle`, { method: 'POST' });
      const payload = await readJson(resp);
      if (resp && resp.ok && payload?.success !== false) {
        notify('success', payload?.message || t('messages.operation_successful', 'Operation successful'));
        setTimeout(loadLists, 800);
      } else {
        notify('warning', payload?.error || t('errors.not_implemented', 'Not implemented'));
      }
    } catch (error) {
      console.error('toggleList error:', error);
      notify('danger', t('errors.network_error', 'Network error. Please try again.'));
    }
  }

  async function toggleDomain(domainName) {
    try {
      const resp = await api(`/api/v1/adblock/domains/${encodeURIComponent(domainName)}/toggle`, { method: 'POST' });
      const payload = await readJson(resp);
      if (resp && resp.ok && payload?.success !== false) {
        notify('success', payload?.message || t('messages.operation_successful', 'Operation successful'));
        setTimeout(loadDomains, 800);
      } else {
        notify('warning', payload?.error || t('errors.not_implemented', 'Not implemented'));
      }
    } catch (error) {
      console.error('toggleDomain error:', error);
      notify('danger', t('errors.network_error', 'Network error. Please try again.'));
    }
  }

  // --- DNSCrypt ---
  async function loadDNSCryptStatus() {
    try {
      const resp = await api('/api/v1/adblock/dnscrypt/status', { method: 'GET' });
      if (!resp || !resp.ok) return;
      const status = await readJson(resp);

      const statusIndicator = document.getElementById('dnscrypt-status-indicator');
      const statusText = document.getElementById('dnscrypt-status-text');
      const installBtn = document.getElementById('dnscrypt-install-btn');
      const configureBtn = document.getElementById('dnscrypt-configure-btn');
      const enableBtn = document.getElementById('dnscrypt-enable-btn');
      const disableBtn = document.getElementById('dnscrypt-disable-btn');

      if (!statusIndicator || !statusText) return;

      if (status?.installed) {
        if (installBtn) installBtn.style.display = 'none';
        if (configureBtn) configureBtn.style.display = 'block';

        if (status?.active) {
          statusIndicator.className = 'status-indicator status-online';
          statusText.textContent = t('dnscrypt.active', 'Active');
          if (enableBtn) enableBtn.style.display = 'none';
          if (disableBtn) disableBtn.style.display = 'block';
        } else {
          statusIndicator.className = 'status-indicator status-offline';
          statusText.textContent = t('dnscrypt.inactive', 'Inactive');
          if (enableBtn) enableBtn.style.display = 'block';
          if (disableBtn) disableBtn.style.display = 'none';
        }
      } else {
        statusIndicator.className = 'status-indicator status-offline';
        statusText.textContent = t('dnscrypt.not_installed', 'Not installed');
        if (installBtn) installBtn.style.display = 'block';
        if (configureBtn) configureBtn.style.display = 'none';
        if (enableBtn) enableBtn.style.display = 'none';
        if (disableBtn) disableBtn.style.display = 'none';
      }
    } catch (error) {
      console.error('Error loading DNSCrypt status:', error);
    }
  }

  async function installDNSCrypt() {
    if (!confirm(`${t('dnscrypt.install', 'Install DNSCrypt')}?`)) return;
    try {
      const resp = await api('/api/v1/adblock/dnscrypt/install', { method: 'POST' });
      const result = await readJson(resp);
      if (resp && resp.ok && result?.success !== false) {
        notify('success', result?.message || t('dnscrypt.installed', 'Installed'));
        loadDNSCryptStatus();
      } else {
        notify('danger', result?.error || t('errors.operation_failed', 'Operation failed'));
      }
    } catch (error) {
      console.error('Error installing DNSCrypt:', error);
      notify('danger', t('errors.network_error', 'Network error. Please try again.'));
    }
  }

  function showDNSCryptConfig() {
    const form = document.getElementById('dnscrypt-config-form');
    if (!form) return;
    form.style.display = form.style.display === 'none' ? 'block' : 'none';
  }

  async function enableDNSCrypt() {
    try {
      const resp = await api('/api/v1/adblock/dnscrypt/enable', { method: 'POST' });
      const result = await readJson(resp);
      if (resp && resp.ok && result?.success !== false) {
        notify('success', result?.message || t('dnscrypt.enabled', 'Enabled'));
        loadDNSCryptStatus();
      } else {
        notify('danger', result?.error || t('errors.operation_failed', 'Operation failed'));
      }
    } catch (error) {
      console.error('Error enabling DNSCrypt:', error);
      notify('danger', t('errors.network_error', 'Network error. Please try again.'));
    }
  }

  async function disableDNSCrypt() {
    if (!confirm(`${t('dnscrypt.disable', 'Disable')} DNSCrypt?`)) return;
    try {
      const resp = await api('/api/v1/adblock/dnscrypt/disable', { method: 'POST' });
      const result = await readJson(resp);
      if (resp && resp.ok && result?.success !== false) {
        notify('success', result?.message || t('dnscrypt.disabled', 'Disabled'));
        loadDNSCryptStatus();
      } else {
        notify('danger', result?.error || t('errors.operation_failed', 'Operation failed'));
      }
    } catch (error) {
      console.error('Error disabling DNSCrypt:', error);
      notify('danger', t('errors.network_error', 'Network error. Please try again.'));
    }
  }

  // --- Forms ---
  function bindForms() {
    document.getElementById('adblockConfigForm')?.addEventListener('submit', async function (e) {
      e.preventDefault();
      const formData = new FormData(this);
      const data = {
        update_interval: formData.get('update_interval'),
        max_lists: parseInt(String(formData.get('max_lists') || '0'), 10),
        cache_size: parseInt(String(formData.get('cache_size') || '0'), 10),
      };

      try {
        const resp = await api('/api/v1/adblock/config', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify(data),
        });
        const payload = await readJson(resp);
        if (resp && resp.ok && payload?.success !== false) {
          notify('success', payload?.message || t('messages.changes_saved', 'Changes saved'));
        } else {
          notify('warning', payload?.error || t('errors.not_implemented', 'Not implemented'));
        }
      } catch (error) {
        console.error('Error saving AdBlock config:', error);
        notify('danger', t('errors.network_error', 'Network error. Please try again.'));
      }
    });

    document.getElementById('dnscryptConfigForm')?.addEventListener('submit', async function (e) {
      e.preventDefault();
      const formData = new FormData(this);
      const data = {
        server_name: formData.get('server_name') || 'adguard-dns',
        block_ads: formData.get('block_ads') === 'on',
      };

      try {
        const resp = await api('/api/v1/adblock/dnscrypt/configure', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify(data),
        });
        const result = await readJson(resp);
        if (resp && resp.ok && result?.success !== false) {
          notify('success', result?.message || t('dnscrypt.configured', 'Configured'));
          document.getElementById('dnscrypt-config-form')?.style && (document.getElementById('dnscrypt-config-form').style.display = 'none');
        } else {
          notify('danger', result?.error || t('errors.operation_failed', 'Operation failed'));
        }
      } catch (error) {
        console.error('Error configuring DNSCrypt:', error);
        notify('danger', t('errors.network_error', 'Network error. Please try again.'));
      }
    });
  }

  // Exponer funciones usadas por onclick en templates
  window.loadLists = loadLists;
  window.loadDomains = loadDomains;
  window.toggleAdBlock = toggleAdBlock;
  window.updateLists = updateLists;
  window.toggleList = toggleList;
  window.toggleDomain = toggleDomain;

  window.loadDNSCryptStatus = loadDNSCryptStatus;
  window.installDNSCrypt = installDNSCrypt;
  window.showDNSCryptConfig = showDNSCryptConfig;
  window.enableDNSCrypt = enableDNSCrypt;
  window.disableDNSCrypt = disableDNSCrypt;

  document.addEventListener('DOMContentLoaded', () => {
    loadLists();
    loadDomains();
    loadDNSCryptStatus();
    bindForms();

    // Auto-refresh (suave)
    setInterval(() => {
      loadLists();
      loadDomains();
      loadDNSCryptStatus();
    }, 30000);
  });
})();

