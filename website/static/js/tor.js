// Tor page logic (migrado desde script inline)
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

  async function loadTorStatus() {
    try {
      const resp = await api('/api/v1/tor/status', { method: 'GET' });
      if (!resp || !resp.ok) return;
      const status = await readJson(resp);

      const statusIndicator = document.getElementById('tor-status-indicator');
      const statusLabel = document.getElementById('tor-status-label');
      const statusText = document.getElementById('tor-status-text');
      const installBtn = document.getElementById('tor-install-btn');
      const configureBtn = document.getElementById('tor-configure-btn');
      const enableBtn = document.getElementById('tor-enable-btn');
      const disableBtn = document.getElementById('tor-disable-btn');
      const installedIcon = document.getElementById('tor-installed-icon');
      const notInstalledIcon = document.getElementById('tor-not-installed-icon');
      const installationText = document.getElementById('tor-installation-text');
      const serviceIndicator = document.getElementById('tor-service-indicator');
      const serviceText = document.getElementById('tor-service-text');
      const torIpText = document.getElementById('tor-ip-text');
      const circuitInfo = document.getElementById('tor-circuit-info');
      const socksPortText = document.getElementById('tor-socks-port-text');
      const socksPortInput = document.getElementById('tor-socks-port');
      const iptablesIndicator = document.getElementById('tor-iptables-indicator');
      const iptablesLabel = document.getElementById('tor-iptables-label');
      const iptablesInterface = document.getElementById('tor-iptables-interface');
      const iptablesEnableBtn = document.getElementById('tor-iptables-enable-btn');
      const iptablesDisableBtn = document.getElementById('tor-iptables-disable-btn');

      if (status?.installed) {
        if (installedIcon) installedIcon.style.display = 'inline';
        if (notInstalledIcon) notInstalledIcon.style.display = 'none';
        if (installationText) installationText.textContent = t('tor.installed', 'Installed');
        if (installBtn) installBtn.style.display = 'none';
        if (configureBtn) configureBtn.style.display = 'block';

        if (status?.active) {
          if (statusIndicator) statusIndicator.className = 'status-indicator status-online';
          if (statusLabel) statusLabel.textContent = t('tor.active', 'Active');
          if (statusText) statusText.textContent = t('tor.active', 'Active');
          if (serviceIndicator) serviceIndicator.className = 'status-indicator status-online';
          if (serviceText) serviceText.textContent = t('tor.running', 'Running');
          if (enableBtn) enableBtn.style.display = 'none';
          if (disableBtn) disableBtn.style.display = 'block';
          if (circuitInfo) circuitInfo.style.display = 'block';

          if (torIpText) torIpText.textContent = status?.tor_ip ? String(status.tor_ip) : '--';

          if (status?.socks_listening) {
            const sp = String(status?.socks_port || '9050');
            if (socksPortText) socksPortText.textContent = sp;
            if (socksPortInput) socksPortInput.value = sp;
          }
          if (iptablesInterface) iptablesInterface.textContent = status?.iptables_interface || 'ap0';
          if (status?.iptables_active) {
            if (iptablesIndicator) iptablesIndicator.className = 'status-indicator status-online';
            if (iptablesLabel) iptablesLabel.textContent = t('tor.torify_active', 'Active');
            if (iptablesEnableBtn) iptablesEnableBtn.style.display = 'none';
            if (iptablesDisableBtn) iptablesDisableBtn.style.display = 'inline-block';
          } else {
            if (iptablesIndicator) iptablesIndicator.className = 'status-indicator status-offline';
            if (iptablesLabel) iptablesLabel.textContent = t('tor.torify_inactive', 'Inactive');
            if (iptablesEnableBtn) iptablesEnableBtn.style.display = 'inline-block';
            if (iptablesDisableBtn) iptablesDisableBtn.style.display = 'none';
          }
        } else {
          if (statusIndicator) statusIndicator.className = 'status-indicator status-offline';
          if (statusLabel) statusLabel.textContent = t('tor.inactive', 'Inactive');
          if (statusText) statusText.textContent = t('tor.inactive', 'Inactive');
          if (serviceIndicator) serviceIndicator.className = 'status-indicator status-offline';
          if (serviceText) serviceText.textContent = t('tor.stopped', 'Stopped');
          if (enableBtn) enableBtn.style.display = 'block';
          if (disableBtn) disableBtn.style.display = 'none';
          if (circuitInfo) circuitInfo.style.display = 'none';
          if (torIpText) torIpText.textContent = '--';
        }
      } else {
        if (installedIcon) installedIcon.style.display = 'none';
        if (notInstalledIcon) notInstalledIcon.style.display = 'inline';
        if (installationText) installationText.textContent = t('tor.not_installed', 'Not installed');
        if (statusIndicator) statusIndicator.className = 'status-indicator status-offline';
        if (statusLabel) statusLabel.textContent = t('tor.not_installed', 'Not installed');
        if (statusText) statusText.textContent = t('tor.not_installed', 'Not installed');
        if (installBtn) installBtn.style.display = 'block';
        if (configureBtn) configureBtn.style.display = 'none';
        if (enableBtn) enableBtn.style.display = 'none';
        if (disableBtn) disableBtn.style.display = 'none';
        if (circuitInfo) circuitInfo.style.display = 'none';
        if (torIpText) torIpText.textContent = '--';
        if (iptablesIndicator) iptablesIndicator.className = 'status-indicator status-offline';
        if (iptablesLabel) iptablesLabel.textContent = t('tor.torify_inactive', 'Inactive');
        if (iptablesEnableBtn) iptablesEnableBtn.style.display = 'none';
        if (iptablesDisableBtn) iptablesDisableBtn.style.display = 'none';
      }
      if (!status?.installed || !status?.active) {
        if (iptablesEnableBtn) iptablesEnableBtn.style.display = 'none';
        if (iptablesDisableBtn) iptablesDisableBtn.style.display = 'none';
        if (iptablesIndicator) iptablesIndicator.className = 'status-indicator status-offline';
        if (iptablesLabel) iptablesLabel.textContent = t('tor.torify_inactive', 'Inactive');
      }
    } catch (error) {
      console.error('Error loading Tor status:', error);
    }
  }

  async function enableTorIptables() {
    try {
      const resp = await api('/api/v1/tor/iptables-enable', { method: 'POST' });
      const result = await readJson(resp);
      if (resp && resp.ok && result?.success !== false) {
        notify('success', result?.message || t('tor.torify_enabled', 'Network traffic now goes through Tor'));
        loadTorStatus();
      } else {
        notify('danger', result?.error || t('errors.operation_failed', 'Operation failed'));
      }
    } catch (error) {
      console.error('Error enabling Tor iptables:', error);
      notify('danger', t('errors.network_error', 'Network error. Please try again.'));
    }
  }

  async function disableTorIptables() {
    try {
      const resp = await api('/api/v1/tor/iptables-disable', { method: 'POST' });
      const result = await readJson(resp);
      if (resp && resp.ok && result?.success !== false) {
        notify('success', result?.message || t('tor.torify_disabled', 'Redirect disabled'));
        loadTorStatus();
      } else {
        notify('danger', result?.error || t('errors.operation_failed', 'Operation failed'));
      }
    } catch (error) {
      console.error('Error disabling Tor iptables:', error);
      notify('danger', t('errors.network_error', 'Network error. Please try again.'));
    }
  }

  async function installTor() {
    if (!confirm(`${t('tor.install', 'Install Tor')}?`)) return;
    try {
      const resp = await api('/api/v1/tor/install', { method: 'POST' });
      const result = await readJson(resp);
      if (resp && resp.ok && result?.success !== false) {
        notify('success', result?.message || t('tor.installed', 'Installed'));
        loadTorStatus();
      } else {
        notify('danger', result?.error || t('errors.operation_failed', 'Operation failed'));
      }
    } catch (error) {
      console.error('Error installing Tor:', error);
      notify('danger', t('errors.network_error', 'Network error. Please try again.'));
    }
  }

  function showTorConfig() {
    const form = document.getElementById('tor-config-form');
    if (!form) return;
    form.style.display = form.style.display === 'none' ? 'block' : 'none';
  }

  async function enableTor() {
    try {
      const resp = await api('/api/v1/tor/enable', { method: 'POST' });
      const result = await readJson(resp);
      if (resp && resp.ok && result?.success !== false) {
        notify('success', result?.message || t('tor.enabled', 'Enabled'));
        setTimeout(() => {
          loadTorStatus();
          loadTorCircuit();
        }, 2500);
      } else {
        notify('danger', result?.error || t('errors.operation_failed', 'Operation failed'));
      }
    } catch (error) {
      console.error('Error enabling Tor:', error);
      notify('danger', t('errors.network_error', 'Network error. Please try again.'));
    }
  }

  async function disableTor() {
    if (!confirm(`${t('tor.disable', 'Disable')} Tor?`)) return;
    try {
      const resp = await api('/api/v1/tor/disable', { method: 'POST' });
      const result = await readJson(resp);
      if (resp && resp.ok && result?.success !== false) {
        notify('success', result?.message || t('tor.disabled', 'Disabled'));
        loadTorStatus();
      } else {
        notify('danger', result?.error || t('errors.operation_failed', 'Operation failed'));
      }
    } catch (error) {
      console.error('Error disabling Tor:', error);
      notify('danger', t('errors.network_error', 'Network error. Please try again.'));
    }
  }

  async function loadTorCircuit() {
    try {
      const resp = await api('/api/v1/tor/circuit', { method: 'GET' });
      if (!resp || !resp.ok) return;
      const circuit = await readJson(resp);
      const circuitText = document.getElementById('tor-circuit-text');
      if (!circuitText) return;

      if (circuit?.tor_ip) {
        circuitText.textContent = `Tor IP: ${circuit.tor_ip}\n${circuit.circuit_info || t(
          'tor.circuit_unavailable',
          'Circuit information not available'
        )}`;
      } else {
        circuitText.textContent =
          circuit?.circuit_info || t('tor.circuit_unavailable', 'Circuit information not available');
      }
    } catch (error) {
      console.error('Error loading Tor circuit:', error);
    }
  }

  function bindTorConfigForm() {
    document.getElementById('torConfigForm')?.addEventListener('submit', async function (e) {
      e.preventDefault();
      const formData = new FormData(this);
      const data = {
        enable_socks: formData.get('enable_socks') === 'on',
        socks_port: parseInt(String(formData.get('socks_port') || '9050'), 10) || 9050,
        enable_control_port: formData.get('enable_control_port') === 'on',
        control_port: parseInt(String(formData.get('control_port') || '9051'), 10) || 9051,
        enable_hidden_service: formData.get('enable_hidden_service') === 'on',
        enable_trans_port: formData.get('enable_trans_port') === 'on',
        trans_port: parseInt(String(formData.get('trans_port') || '9040'), 10) || 9040,
        enable_dns_port: formData.get('enable_dns_port') === 'on',
        dns_port: parseInt(String(formData.get('dns_port') || '53'), 10) || 53,
        client_only: formData.get('client_only') === 'on',
        automap_hosts_on_resolve: formData.get('automap_hosts_on_resolve') === 'on',
      };

      try {
        const resp = await api('/api/v1/tor/configure', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify(data),
        });
        const result = await readJson(resp);
        if (resp && resp.ok && result?.success !== false) {
          notify('success', result?.message || t('tor.configured', 'Configured'));
          const el = document.getElementById('tor-config-form');
          if (el) el.style.display = 'none';
        } else {
          notify('danger', result?.error || t('errors.operation_failed', 'Operation failed'));
        }
      } catch (error) {
        console.error('Error configuring Tor:', error);
        notify('danger', t('errors.network_error', 'Network error. Please try again.'));
      }
    });
  }

  // Exponer funciones usadas por onclick en templates
  window.loadTorStatus = loadTorStatus;
  window.installTor = installTor;
  window.showTorConfig = showTorConfig;
  window.enableTor = enableTor;
  window.disableTor = disableTor;
  window.loadTorCircuit = loadTorCircuit;
  window.enableTorIptables = enableTorIptables;
  window.disableTorIptables = disableTorIptables;

  document.addEventListener('DOMContentLoaded', function () {
    bindTorConfigForm();
    loadTorStatus();
    loadTorCircuit();
    setInterval(() => {
      loadTorStatus();
      loadTorCircuit();
    }, 30000);
  });
})();

