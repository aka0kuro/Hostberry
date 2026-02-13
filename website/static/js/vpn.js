// VPN page
(function () {
  const api = (url, opts) => (window.HostBerry?.apiRequest ? window.HostBerry.apiRequest(url, opts) : fetch(url, opts));
  const t = (key, fallback) => (window.HostBerry?.t ? window.HostBerry.t(key, fallback) : fallback || key);
  const showAlert = (type, msg) => (window.HostBerry?.showAlert ? window.HostBerry.showAlert(type, msg) : alert(msg));

  function escapeHtml(s) {
    const str = String(s ?? '');
    return str.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;').replace(/'/g, '&#39;');
  }

  async function loadConnections() {
    try {
      const resp = await api('/api/v1/vpn/connections');
      if (resp?.ok) {
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
      if (resp?.ok) {
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
      if (resp?.ok) {
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
      if (resp?.ok) {
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
      if (resp?.ok) {
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
      if (resp?.ok) {
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
      if (resp?.ok) {
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

  const cfgForm = document.getElementById('vpnConfigForm');
  if (cfgForm) {
    cfgForm.addEventListener('submit', async function (e) {
      e.preventDefault();
      const fd = new FormData(this);
      const data = {
        server_name: fd.get('server_name'),
        server_address: fd.get('server_address'),
        server_port: parseInt(fd.get('server_port'), 10),
        protocol: fd.get('protocol'),
        encryption: fd.get('encryption'),
      };
      try {
        const resp = await api('/api/v1/vpn/config', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify(data),
        });
        if (resp?.ok) {
          showAlert('success', t('messages.changes_saved', 'Changes saved'));
        } else {
          showAlert('danger', t('errors.configuration_error', 'Configuration error'));
        }
      } catch (_e) {
        showAlert('danger', t('errors.network_error', 'Network error'));
      }
    });
  }

  document.addEventListener('DOMContentLoaded', function () {
    loadConnections();
    loadServers();
    loadClients();
    setInterval(function () {
      loadConnections();
      loadServers();
      loadClients();
    }, 30000);
  });

  window.toggleVPN = toggleVPN;
  window.connectVPN = connectVPN;
  window.toggleConnection = toggleConnection;
  window.generateCertificates = generateCertificates;
  window.viewSecurityLogs = viewSecurityLogs;
})();
