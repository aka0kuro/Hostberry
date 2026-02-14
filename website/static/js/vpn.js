// VPN page
(function () {
  const api = (url, opts) => (window.HostBerry?.apiRequest ? window.HostBerry.apiRequest(url, opts) : fetch(url, opts));
  const t = (key, fallback) => (window.HostBerry?.t ? window.HostBerry.t(key, fallback) : fallback || key);
  const showAlert = (type, msg) => (window.HostBerry?.showAlert ? window.HostBerry.showAlert(type, msg) : alert(msg));

  function escapeHtml(s) {
    const str = String(s ?? '');
    return (window.HostBerry && window.HostBerry.escapeHtml) ? window.HostBerry.escapeHtml(str) : str.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;').replace(/'/g, '&#39;');
  }

  async function loadConnections() {
    try {
      const resp = await api('/api/v1/vpn/connections');
      if (resp && resp.ok) {
        const connections = await resp.json();
        const tbody = document.getElementById('connectionsTable');
        if (!tbody) return;
        tbody.innerHTML = '';
        (Array.isArray(connections) ? connections : []).forEach(function (conn) {
          const tr = document.createElement('tr');
          const name = escapeHtml(conn?.name ?? '');
          const type = escapeHtml(conn?.type ?? '');
          const statusText = conn?.status === 'connected' ? t('vpn.connected', 'Connected') : t('vpn.disconnected', 'Disconnected');
          const bandwidth = escapeHtml(conn?.bandwidth ?? '');
          tr.innerHTML =
            '<td>' + name + '</td><td>' + type + '</td>' +
            '<td><span class="badge bg-' + (conn?.status === 'connected' ? 'success' : 'danger') + '">' + escapeHtml(statusText) + '</span></td>' +
            '<td>' + bandwidth + '</td>' +
            '<td><button class="btn btn-sm btn-outline-primary" type="button"><i class="bi bi-' + (conn?.status === 'connected' ? 'pause' : 'play') + '"></i></button></td>';
          const toggleBtn = tr.querySelector('button');
          if (toggleBtn) {
            toggleBtn.addEventListener('click', () => toggleConnection(conn?.name ?? ''));
          }
          tbody.appendChild(tr);
        });
      }
    } catch (e) {
      console.error('Error loading connections:', e);
    }
  }

  async function loadServers() {
    try {
      const resp = await api('/api/v1/vpn/servers');
      if (resp && resp.ok) {
        const servers = await resp.json();
        const tbody = document.getElementById('serversTable');
        if (!tbody) return;
        tbody.innerHTML = '';
        (Array.isArray(servers) ? servers : []).forEach(function (server) {
          const tr = document.createElement('tr');
          tr.innerHTML =
            '<td>' + escapeHtml(server?.name ?? '') + '</td>' +
            '<td>' + escapeHtml(server?.address ?? '') + '</td>' +
            '<td><span class="badge bg-' + (server?.status === 'running' ? 'success' : 'danger') + '">' + escapeHtml(server?.status ?? '') + '</span></td>' +
            '<td>' + escapeHtml(server?.clients_count ?? '') + '</td>';
          tbody.appendChild(tr);
        });
      }
    } catch (e) {
      console.error('Error loading servers:', e);
    }
  }

  async function loadClients() {
    try {
      const resp = await api('/api/v1/vpn/clients');
      if (resp && resp.ok) {
        const clients = await resp.json();
        const tbody = document.getElementById('clientsTable');
        if (!tbody) return;
        tbody.innerHTML = '';
        (Array.isArray(clients) ? clients : []).forEach(function (client) {
          const tr = document.createElement('tr');
          const statusText = client?.connected ? t('vpn.connected', 'Connected') : t('vpn.disconnected', 'Disconnected');
          tr.innerHTML =
            '<td>' + escapeHtml(client?.name ?? '') + '</td>' +
            '<td>' + escapeHtml(client?.address ?? '') + '</td>' +
            '<td><span class="badge bg-' + (client?.connected ? 'success' : 'danger') + '">' + escapeHtml(statusText) + '</span></td>' +
            '<td>' + escapeHtml(client?.bandwidth ?? '') + '</td>';
          tbody.appendChild(tr);
        });
      }
    } catch (e) {
      console.error('Error loading clients:', e);
    }
  }

  async function toggleVPN() {
    try {
      const resp = await api('/api/v1/vpn/toggle', { method: 'POST' });
      if (resp && resp.ok) {
        showAlert('success', t('messages.operation_successful', 'Operation successful'));
        setTimeout(() => window.location.reload(), 1000);
      } else {
        showAlert('danger', t('errors.operation_failed', 'Operation failed'));
      }
    } catch (_e) {
      showAlert('danger', t('errors.network_error', 'Network error'));
    }
  }

  async function connectVPN() {
    try {
      const resp = await api('/api/v1/vpn/connect', { method: 'POST' });
      if (resp && resp.ok) {
        showAlert('success', t('messages.operation_successful', 'Operation successful'));
        setTimeout(() => window.location.reload(), 1000);
      } else {
        showAlert('danger', t('errors.operation_failed', 'Operation failed'));
      }
    } catch (_e) {
      showAlert('danger', t('errors.network_error', 'Network error'));
    }
  }

  async function toggleConnection(name) {
    try {
      const safeName = encodeURIComponent(String(name ?? ''));
      const resp = await api('/api/v1/vpn/connections/' + safeName + '/toggle', { method: 'POST' });
      if (resp && resp.ok) {
        showAlert('success', t('messages.operation_successful', 'Operation successful'));
        setTimeout(loadConnections, 1000);
      } else {
        showAlert('danger', t('errors.operation_failed', 'Operation failed'));
      }
    } catch (_e) {
      showAlert('danger', t('errors.network_error', 'Network error'));
    }
  }

  async function generateCertificates() {
    try {
      const resp = await api('/api/v1/vpn/certificates/generate', { method: 'POST' });
      if (resp && resp.ok) {
        showAlert('success', t('messages.operation_successful', 'Operation successful'));
      } else {
        showAlert('danger', t('errors.operation_failed', 'Operation failed'));
      }
    } catch (_e) {
      showAlert('danger', t('errors.network_error', 'Network error'));
    }
  }

  function viewSecurityLogs() {
    window.location.href = '/system#system-logs';
  }

  async function loadOpenVPNConfig() {
    try {
      const resp = await api('/api/v1/vpn/config');
      if (resp && resp.ok) {
        const data = await resp.json();
        const ta = document.getElementById('openvpn_config');
        if (ta && data && typeof data.config === 'string') ta.value = data.config;
      }
    } catch (e) {
      console.error('Error loading OpenVPN config:', e);
    }
  }

  async function saveOpenVPNConfig() {
    const ta = document.getElementById('openvpn_config');
    const config = (ta && ta.value) ? ta.value.trim() : '';
    if (!config) {
      showAlert('warning', t('vpn.config_required', 'Paste or upload a configuration first'));
      return;
    }
    try {
      const resp = await api('/api/v1/vpn/config', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ config: config }),
      });
      const result = await resp.json().catch(() => ({}));
      if (resp && resp.ok && result && result.success !== false) {
        showAlert('success', result.message || t('messages.changes_saved', 'Changes saved'));
      } else {
        showAlert('danger', result.error || t('errors.configuration_error', 'Configuration error'));
      }
    } catch (_e) {
      showAlert('danger', t('errors.network_error', 'Network error'));
    }
  }

  async function connectOpenVPN() {
    const ta = document.getElementById('openvpn_config');
    const config = (ta && ta.value) ? ta.value.trim() : '';
    if (!config) {
      showAlert('warning', t('vpn.config_required', 'Paste or upload a configuration first'));
      return;
    }
    try {
      const resp = await api('/api/v1/vpn/connect', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ config: config, type: 'openvpn' }),
      });
      const result = await resp.json().catch(() => ({}));
      if (resp && resp.ok && result && result.success !== false) {
        showAlert('success', result.message || t('vpn.connect_vpn', 'Connect'));
        setTimeout(() => window.location.reload(), 1500);
      } else {
        showAlert('danger', result.error || t('errors.operation_failed', 'Operation failed'));
      }
    } catch (_e) {
      showAlert('danger', t('errors.network_error', 'Network error'));
    }
  }

  const openvpnFileInput = document.getElementById('openvpn_config_file');
  if (openvpnFileInput) {
    openvpnFileInput.addEventListener('change', function () {
      const file = this.files && this.files[0];
      if (!file) return;
      const reader = new FileReader();
      reader.onload = function () {
        const ta = document.getElementById('openvpn_config');
        if (ta) ta.value = reader.result || '';
      };
      reader.readAsText(file);
    });
  }

  document.addEventListener('DOMContentLoaded', function () {
    loadConnections();
    loadServers();
    loadClients();
    loadOpenVPNConfig();
    setInterval(function () {
      loadConnections();
      loadServers();
      loadClients();
    }, 30000);
  });

  window.toggleVPN = toggleVPN;
  window.connectVPN = connectVPN;
  window.connectOpenVPN = connectOpenVPN;
  window.saveOpenVPNConfig = saveOpenVPNConfig;
  window.toggleConnection = toggleConnection;
  window.generateCertificates = generateCertificates;
  window.viewSecurityLogs = viewSecurityLogs;
})();
