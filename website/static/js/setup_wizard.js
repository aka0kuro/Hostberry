(function() {
  const t = window.HostBerry && window.HostBerry.t ? function(k, d) { return HostBerry.t(k, d); } : function(_, d) { return d || _; };
  const showAlert = window.HostBerry && window.HostBerry.showAlert ? function(type, msg) { HostBerry.showAlert(type, msg); } : function(_, msg) { alert(msg); };
  const apiRequest = window.HostBerry && window.HostBerry.apiRequest ? function(u, o) { return HostBerry.apiRequest(u, o); } : function(u, o) { return fetch(u, Object.assign({ credentials: 'include' }, o)); };

  let selectedSSID = null;
  let wizardStep = 1;

  function setStep(step) {
    wizardStep = step;
    document.querySelectorAll('.setup-step').forEach(function(el) {
      el.classList.toggle('d-none', parseInt(el.getAttribute('data-step'), 10) !== step);
    });
    document.querySelectorAll('.step-dot').forEach(function(el) {
      el.classList.toggle('active', parseInt(el.getAttribute('data-step'), 10) === step);
    });
  }

  function fillNetworksList(networks) {
    const list = document.getElementById('wizard-networks-list');
    if (!list) return;
    list.innerHTML = '';
    if (!Array.isArray(networks) || networks.length === 0) {
      list.innerHTML = '<div class="list-group-item text-muted">' + t('setup_wizard.select_network', 'Selecciona una red') + '</div>';
      return;
    }
    networks.forEach(function(net) {
      const ssid = net.ssid || net.SSID || '';
      if (!ssid) return;
      const signal = net.signal || net.signal_strength || '';
      const item = document.createElement('button');
      item.type = 'button';
      item.className = 'list-group-item list-group-item-action d-flex justify-content-between align-items-center';
      item.dataset.ssid = ssid;
      item.innerHTML = '<span>' + (window.HostBerry && HostBerry.escapeHtml ? HostBerry.escapeHtml(ssid) : ssid) + '</span>' + (signal ? '<small class="text-muted">' + signal + '</small>' : '');
      item.addEventListener('click', function() {
        selectedSSID = ssid;
        list.querySelectorAll('.list-group-item-action').forEach(function(i) { i.classList.remove('active'); });
        item.classList.add('active');
        document.getElementById('wizard-wifi-password-box').classList.remove('d-none');
        document.getElementById('wizard-wifi-password').value = '';
        document.getElementById('wizard-wifi-password').focus();
      });
      list.appendChild(item);
    });
  }

  async function scanNetworks() {
    const btn = document.getElementById('wizard-scan-btn');
    if (btn) {
      btn.disabled = true;
      btn.innerHTML = '<i class="bi bi-arrow-clockwise spinning me-2"></i>' + t('common.loading', 'Cargando...');
    }
    try {
      const resp = await apiRequest('/api/v1/wifi/scan', { method: 'POST' });
      const data = resp.ok ? await resp.json() : [];
      fillNetworksList(Array.isArray(data) ? data : (data.networks || []));
    } catch (e) {
      showAlert('danger', t('setup_wizard.error_scan', 'Error al buscar redes'));
      fillNetworksList([]);
    } finally {
      if (btn) {
        btn.disabled = false;
        btn.innerHTML = '<i class="bi bi-arrow-clockwise me-2"></i>' + t('setup_wizard.scan_networks', 'Buscar redes');
      }
    }
  }

  async function connectWiFi() {
    if (!selectedSSID) return;
    const password = (document.getElementById('wizard-wifi-password') || {}).value || '';
    const connectBtn = document.getElementById('wizard-connect-btn');
    if (connectBtn) {
      connectBtn.disabled = true;
      connectBtn.querySelector('.btn-text').textContent = t('setup_wizard.connecting', 'Conectando...');
    }
    try {
      const resp = await apiRequest('/api/v1/wifi/connect', {
        method: 'POST',
        body: { ssid: selectedSSID, password: password, country: 'ES' }
      });
      const data = await resp.json().catch(function() { return {}; });
      if (resp.ok && data.success !== false) {
        showAlert('success', t('setup_wizard.connected', 'Conectado'));
        document.getElementById('wizard-next-1').disabled = false;
      } else {
        showAlert('danger', (data.error || t('setup_wizard.error_connect', 'Error al conectar')));
      }
    } catch (e) {
      showAlert('danger', t('setup_wizard.error_connect', 'Error al conectar'));
    } finally {
      if (connectBtn) {
        connectBtn.disabled = false;
        connectBtn.querySelector('.btn-text').textContent = t('setup_wizard.connect', 'Conectar');
      }
    }
  }

  async function saveHostapd() {
    const ssid = (document.getElementById('wizard-ap-ssid') || {}).value || 'hostberry';
    const open = (document.getElementById('wizard-ap-open') || {}).checked;
    const password = (document.getElementById('wizard-ap-password') || {}).value || '';
    const saveBtn = document.getElementById('wizard-save-ap');
    if (saveBtn) {
      saveBtn.disabled = true;
      saveBtn.querySelector('.btn-text').textContent = t('common.saving', 'Guardando...');
    }
    const payload = {
      interface: 'wlan0',
      ssid: ssid,
      channel: 6,
      security: open ? 'open' : 'wpa2',
      password: open ? '' : password,
      gateway: '192.168.4.1',
      dhcp_range_start: '192.168.4.2',
      dhcp_range_end: '192.168.4.254',
      lease_time: '12h',
      country: 'ES'
    };
    try {
      const resp = await apiRequest('/api/v1/hostapd/config', { method: 'POST', body: payload });
      const data = await resp.json().catch(function() { return {}; });
      if (resp.ok && !data.error) {
        showAlert('success', t('setup_wizard.success_ap', 'Punto de acceso configurado correctamente'));
        setTimeout(function() { window.location.href = '/dashboard'; }, 1500);
      } else {
        showAlert('danger', data.error || t('setup_wizard.error_save_ap', 'Error al guardar la configuración del punto de acceso'));
        if (saveBtn) { saveBtn.disabled = false; saveBtn.querySelector('.btn-text').textContent = t('setup_wizard.save_and_finish', 'Guardar y finalizar'); }
      }
    } catch (e) {
      showAlert('danger', t('setup_wizard.error_save_ap', 'Error al guardar la configuración del punto de acceso'));
      if (saveBtn) { saveBtn.disabled = false; saveBtn.querySelector('.btn-text').textContent = t('setup_wizard.save_and_finish', 'Guardar y finalizar'); }
    }
  }

  function init() {
    document.getElementById('wizard-scan-btn').addEventListener('click', scanNetworks);
    document.getElementById('wizard-connect-btn').addEventListener('click', connectWiFi);
    document.getElementById('wizard-next-1').addEventListener('click', function() { setStep(2); });
    document.getElementById('wizard-back-2').addEventListener('click', function() { setStep(1); });
    document.getElementById('wizard-save-ap').addEventListener('click', saveHostapd);

    document.getElementById('wizard-ap-open').addEventListener('change', function() {
      document.getElementById('wizard-ap-password-box').classList.toggle('d-none', this.checked);
    });

    setStep(1);
    scanNetworks();
  }

  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', init);
  } else {
    init();
  }
})();
