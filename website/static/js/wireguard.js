// WireGuard page
(function () {
  const api = (url, opts) => (window.HostBerry?.apiRequest ? window.HostBerry.apiRequest(url, opts) : fetch(url, opts));
  const t = (key, fallback) => (window.HostBerry?.t ? window.HostBerry.t(key, fallback) : fallback || key);
  const showAlert = (type, msg) => (window.HostBerry?.showAlert ? window.HostBerry.showAlert(type, msg) : alert(msg));

  function escapeHtml(s) {
    const str = String(s ?? '');
    return (window.HostBerry && window.HostBerry.escapeHtml) ? window.HostBerry.escapeHtml(str) : str.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;').replace(/'/g, '&#39;');
  }

  async function loadConfig() {
    try {
      const resp = await api('/api/v1/wireguard/config');
      if (resp && resp.ok) {
        const data = await resp.json();
        const ta = document.getElementById('wg_config');
        if (ta && data && typeof data.config === 'string') ta.value = data.config;
      }
    } catch (e) {
      console.error('Error loading wireguard config:', e);
    }
  }

  async function loadInterfaces() {
    try {
      const resp = await api('/api/v1/wireguard/interfaces');
      if (resp && resp.ok) {
        const interfaces = await resp.json();
        const tbody = document.getElementById('interfacesTable');
        if (!tbody) return;
        tbody.innerHTML = '';
        (Array.isArray(interfaces) ? interfaces : []).forEach(function (iface) {
          const tr = document.createElement('tr');
          const name = escapeHtml(iface?.name ?? '');
          const status = escapeHtml(iface?.status ?? '');
          const address = escapeHtml(iface?.address ?? '');
          const peers = escapeHtml(iface?.peers_count ?? '');
          tr.innerHTML =
            '<td>' + name + '</td>' +
            '<td><span class="badge bg-' + (iface?.status === 'up' ? 'success' : 'danger') + '">' + status + '</span></td>' +
            '<td>' + address + '</td><td>' + peers + '</td>' +
            '<td><button class="btn btn-sm btn-outline-primary" type="button"><i class="bi bi-gear"></i></button></td>';
          const configBtn = tr.querySelector('button');
          if (configBtn) {
            configBtn.addEventListener('click', () => configureInterface(iface?.name ?? ''));
          }
          tbody.appendChild(tr);
        });
      }
    } catch (e) {
      console.error('Error loading interfaces:', e);
    }
  }

  async function loadPeers() {
    try {
      const resp = await api('/api/v1/wireguard/peers');
      if (resp && resp.ok) {
        const peers = await resp.json();
        const tbody = document.getElementById('peersTable');
        if (!tbody) return;
        tbody.innerHTML = '';
        (Array.isArray(peers) ? peers : []).forEach(function (peer) {
          const tr = document.createElement('tr');
          const name = escapeHtml(peer?.name ?? '');
          const statusText = peer?.connected ? t('wireguard.connected', 'Connected') : t('wireguard.disconnected', 'Disconnected');
          const bandwidth = escapeHtml(peer?.bandwidth ?? '');
          const uptime = escapeHtml(peer?.uptime ?? '');
          tr.innerHTML =
            '<td>' + name + '</td>' +
            '<td><span class="badge bg-' + (peer?.connected ? 'success' : 'danger') + '">' + escapeHtml(statusText) + '</span></td>' +
            '<td>' + bandwidth + '</td><td>' + uptime + '</td>';
          tbody.appendChild(tr);
        });
      }
    } catch (e) {
      console.error('Error loading peers:', e);
    }
  }

  async function toggleWireGuard() {
    try {
      const resp = await api('/api/v1/wireguard/toggle', { method: 'POST' });
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

  async function restartWireGuard() {
    try {
      const resp = await api('/api/v1/wireguard/restart', { method: 'POST' });
      if (resp && resp.ok) {
        showAlert('success', t('messages.operation_successful', 'Operation successful'));
        setTimeout(() => window.location.reload(), 2000);
      } else {
        showAlert('danger', t('errors.operation_failed', 'Operation failed'));
      }
    } catch (_e) {
      showAlert('danger', t('errors.network_error', 'Network error'));
    }
  }

  function configureInterface(name) {
    showAlert('info', t('wireguard.configuring_interface', 'Configuring interface') + ': ' + String(name ?? ''));
  }

  const wgFileInput = document.getElementById('wg_config_file');
  if (wgFileInput) {
    wgFileInput.addEventListener('change', function () {
      const file = this.files && this.files[0];
      if (!file) return;
      const reader = new FileReader();
      reader.onload = function () {
        const ta = document.getElementById('wg_config');
        if (ta) ta.value = reader.result || '';
      };
      reader.readAsText(file);
    });
  }

  const cfgForm = document.getElementById('wireguardConfigForm');
  if (cfgForm) {
    cfgForm.addEventListener('submit', async function (e) {
      e.preventDefault();
      const fd = new FormData(this);
      const data = { config: fd.get('config') };
      try {
        const resp = await api('/api/v1/wireguard/config', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify(data),
        });
        if (resp && resp.ok) {
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
    loadConfig();
    loadInterfaces();
    loadPeers();
    setInterval(function () {
      loadInterfaces();
      loadPeers();
    }, 30000);
  });

  window.toggleWireGuard = toggleWireGuard;
  window.restartWireGuard = restartWireGuard;
  window.configureInterface = configureInterface;
})();
