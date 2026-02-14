(function() {
  const t = window.HostBerry && window.HostBerry.t ? function(k, d) { return HostBerry.t(k, d); } : function(_, d) { return d || _; };
  const showAlert = window.HostBerry && window.HostBerry.showAlert ? function(type, msg) { HostBerry.showAlert(type, msg); } : function(_, msg) { alert(msg); };
  const apiRequest = window.HostBerry && window.HostBerry.apiRequest ? function(u, o) { return HostBerry.apiRequest(u, o); } : function(u, o) { return fetch(u, Object.assign({ credentials: 'include' }, o)); };

  let selectedSSID = null;
  let wizardStep = 1;
  let selectedSecurityOption = null; // 'vpn' | 'wireguard' | 'tor'

  function setStep(step) {
    wizardStep = step;
    document.querySelectorAll('.setup-step').forEach(function(el) {
      el.classList.toggle('d-none', parseInt(el.getAttribute('data-step'), 10) !== step);
    });
    document.querySelectorAll('.step-dot').forEach(function(el) {
      el.classList.toggle('active', parseInt(el.getAttribute('data-step'), 10) === step);
    });
    if (step === 4 && selectedSecurityOption) {
      showStep4Config(selectedSecurityOption);
    }
  }

  function showStep4Config(option) {
    document.querySelectorAll('.wizard-config-panel').forEach(function(p) { p.classList.add('d-none'); });
    var titleText = t('setup_wizard.step_config_title', 'Configuración');
    var desc = '';
    if (option === 'vpn') {
      document.getElementById('wizard-config-vpn').classList.remove('d-none');
      titleText = t('setup_wizard.security_vpn', 'VPN');
      desc = t('setup_wizard.step_security_desc', 'Configura la VPN.');
    } else if (option === 'wireguard') {
      document.getElementById('wizard-config-wireguard').classList.remove('d-none');
      titleText = t('setup_wizard.security_wireguard', 'WireGuard');
      desc = t('setup_wizard.step_security_desc', 'Configura WireGuard.');
    } else if (option === 'tor') {
      document.getElementById('wizard-config-tor').classList.remove('d-none');
      titleText = t('setup_wizard.security_tor', 'Tor');
      desc = t('setup_wizard.step_security_desc', 'Instala y habilita Tor.');
      loadTorStatusWizard();
    }
    var titleEl = document.getElementById('step4-title-text');
    var descEl = document.getElementById('step4-desc');
    if (titleEl) titleEl.textContent = titleText;
    if (descEl) descEl.textContent = desc;
  }

  function signalBars(signal) {
    if (signal == null || signal === '') return '';
    var n = Number(signal);
    if (isNaN(n)) return '';
    if (n >= -50) return '4'; // 4 barras
    if (n >= -60) return '3';
    if (n >= -70) return '2';
    if (n >= -80) return '1';
    return '0';
  }

  function fillNetworksList(networks) {
    const grid = document.getElementById('wizard-networks-grid');
    if (!grid) return;
    grid.innerHTML = '';
    if (!Array.isArray(networks) || networks.length === 0) {
      grid.innerHTML = '<p class="wizard-networks-empty text-muted">' + t('setup_wizard.select_network', 'Selecciona una red') + '</p>';
      return;
    }
    var escapeHtml = window.HostBerry && HostBerry.escapeHtml ? function(s) { return HostBerry.escapeHtml(s); } : function(s) { return s; };
    networks.forEach(function(net) {
      const ssid = net.ssid || net.SSID || '';
      if (!ssid) return;
      const signal = net.signal != null ? net.signal : net.signal_strength;
      const bars = signalBars(signal);
      const card = document.createElement('button');
      card.type = 'button';
      card.className = 'wizard-network-card';
      card.dataset.ssid = ssid;
      card.innerHTML =
        '<span class="wizard-network-icon"><i class="bi bi-wifi"></i><span class="wizard-signal-bars" data-bars="' + bars + '"></span></span>' +
        '<span class="wizard-network-ssid">' + escapeHtml(ssid) + '</span>' +
        (signal !== '' && signal != null ? '<span class="wizard-network-signal">' + signal + ' dBm</span>' : '');
      card.addEventListener('click', function() {
        selectedSSID = ssid;
        grid.querySelectorAll('.wizard-network-card').forEach(function(c) { c.classList.remove('selected'); });
        card.classList.add('selected');
        document.getElementById('wizard-wifi-password-box').classList.remove('d-none');
        document.getElementById('wizard-wifi-password').value = '';
        document.getElementById('wizard-wifi-password').focus();
      });
      grid.appendChild(card);
    });
  }

  function setupPasswordToggle(inputId, toggleBtnId) {
    var input = document.getElementById(inputId);
    var btn = document.getElementById(toggleBtnId);
    if (!input || !btn) return;
    var icon = btn.querySelector('i.bi');
    var showLabel = t('setup_wizard.show_password', 'Ver contraseña');
    var hideLabel = t('setup_wizard.hide_password', 'Ocultar contraseña');
    btn.addEventListener('click', function() {
      if (input.type === 'password') {
        input.type = 'text';
        btn.title = hideLabel;
        btn.setAttribute('aria-label', hideLabel);
        if (icon) { icon.classList.remove('bi-eye'); icon.classList.add('bi-eye-slash'); }
      } else {
        input.type = 'password';
        btn.title = showLabel;
        btn.setAttribute('aria-label', showLabel);
        if (icon) { icon.classList.remove('bi-eye-slash'); icon.classList.add('bi-eye'); }
      }
    });
  }

  async function scanNetworks() {
    const btn = document.getElementById('wizard-scan-btn');
    if (btn) {
      btn.disabled = true;
      btn.innerHTML = '<i class="bi bi-arrow-clockwise spinning me-2"></i>' + t('setup_wizard.scanning', 'Escaneando...');
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
        setStep(2);
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
    const password = (document.getElementById('wizard-ap-password') || {}).value.trim();
    const saveBtn = document.getElementById('wizard-next-2');
    if (!open && (password.length < 8 || password.length > 63)) {
      showAlert('warning', t('setup_wizard.ap_password_invalid', 'La contraseña debe tener entre 8 y 63 caracteres (estándar WiFi WPA2/WPA3).'));
      return;
    }
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
        setStep(3);
      } else {
        showAlert('danger', data.error || t('setup_wizard.error_save_ap', 'Error al guardar la configuración del punto de acceso'));
        if (saveBtn) { saveBtn.disabled = false; saveBtn.querySelector('.btn-text').textContent = t('setup_wizard.next', 'Siguiente'); }
      }
    } catch (e) {
      showAlert('danger', t('setup_wizard.error_save_ap', 'Error al guardar la configuración del punto de acceso'));
      if (saveBtn) { saveBtn.disabled = false; saveBtn.querySelector('.btn-text').textContent = t('setup_wizard.next', 'Siguiente'); }
    }
  }

  function loadTorStatusWizard() {
    var api = window.HostBerry && window.HostBerry.apiRequest ? window.HostBerry.apiRequest.bind(window.HostBerry) : fetch;
    api('/api/v1/tor/status', { method: 'GET' }).then(function(r) { return r.ok ? r.json() : {}; }).then(function(s) {
      var dot = document.getElementById('wizard-tor-status-dot');
      var text = document.getElementById('wizard-tor-status-text');
      var installBtn = document.getElementById('wizard-tor-install');
      var enableBtn = document.getElementById('wizard-tor-enable');
      if (!text) return;
      if (s.installed) {
        if (dot) { dot.className = 'status-indicator ' + (s.active ? 'status-online' : 'status-offline'); }
        text.textContent = s.active ? t('tor.active', 'Activo') : t('tor.inactive', 'Inactivo');
        if (installBtn) installBtn.classList.add('d-none');
        if (enableBtn) { enableBtn.classList.remove('d-none'); enableBtn.textContent = s.active ? t('tor.disable', 'Deshabilitar') : t('tor.enable', 'Habilitar'); }
      } else {
        if (dot) dot.className = 'status-indicator status-offline';
        text.textContent = t('tor.not_installed', 'No instalado');
        if (installBtn) installBtn.classList.remove('d-none');
        if (enableBtn) enableBtn.classList.add('d-none');
      }
    }).catch(function() {});
  }

  async function wizardTorInstall() {
    var btn = document.getElementById('wizard-tor-install');
    if (btn) { btn.disabled = true; btn.textContent = t('common.loading', 'Cargando...'); }
    try {
      var r = await (window.HostBerry && window.HostBerry.apiRequest ? window.HostBerry.apiRequest('/api/v1/tor/install', { method: 'POST' }) : fetch('/api/v1/tor/install', { method: 'POST', credentials: 'include' }));
      var d = await r.json().catch(function() { return {}; });
      if (r.ok && !d.error) { showAlert('success', d.message || t('tor.installed', 'Instalado')); loadTorStatusWizard(); }
      else { showAlert('danger', d.error || t('errors.operation_failed', 'Error')); }
    } catch (e) { showAlert('danger', e.message || 'Error'); }
    if (btn) { btn.disabled = false; btn.textContent = t('tor.install', 'Instalar Tor'); }
  }

  async function wizardTorEnable() {
    var enableBtn = document.getElementById('wizard-tor-enable');
    if (enableBtn) { enableBtn.disabled = true; }
    try {
      var r = await (window.HostBerry && window.HostBerry.apiRequest ? window.HostBerry.apiRequest('/api/v1/tor/enable', { method: 'POST' }) : fetch('/api/v1/tor/enable', { method: 'POST', credentials: 'include' }));
      var d = await r.json().catch(function() { return {}; });
      if (r.ok && !d.error) { showAlert('success', d.message || t('tor.enabled', 'Habilitado')); loadTorStatusWizard(); }
      else { showAlert('danger', d.error || 'Error'); }
    } catch (e) { showAlert('danger', e.message || 'Error'); }
    if (enableBtn) { enableBtn.disabled = false; loadTorStatusWizard(); }
  }

  async function wizardWgSave() {
    var ta = document.getElementById('wizard-wg-config');
    var config = (ta && ta.value) ? ta.value.trim() : '';
    if (!config) { showAlert('warning', t('setup_wizard.wg_config_empty', 'Escribe o pega la configuración.')); return; }
    try {
      var r = await apiRequest('/api/v1/wireguard/config', { method: 'POST', body: { config: config } });
      var d = await r.json().catch(function() { return {}; });
      if (r.ok && !d.error) { showAlert('success', t('common.saved', 'Guardado')); }
      else { showAlert('danger', d.error || 'Error'); }
    } catch (e) { showAlert('danger', e.message || 'Error'); }
  }

  function init() {
    document.getElementById('wizard-scan-btn').addEventListener('click', scanNetworks);
    document.getElementById('wizard-connect-btn').addEventListener('click', connectWiFi);
    document.getElementById('wizard-back-2').addEventListener('click', function() { setStep(1); });
    document.getElementById('wizard-next-2').addEventListener('click', function() { saveHostapd(); });
    document.getElementById('wizard-back-3').addEventListener('click', function() { setStep(2); });
    document.getElementById('wizard-back-4').addEventListener('click', function() { setStep(3); });

    document.querySelectorAll('.wizard-security-option').forEach(function(card) {
      card.addEventListener('click', function() {
        selectedSecurityOption = card.getAttribute('data-option');
        document.querySelectorAll('.wizard-security-option').forEach(function(c) { c.classList.remove('border-primary', 'border-3'); });
        card.classList.add('border-primary', 'border-3');
        document.getElementById('wizard-go-config').disabled = false;
      });
    });
    document.getElementById('wizard-go-config').addEventListener('click', function() {
      if (selectedSecurityOption) setStep(4);
    });

    document.getElementById('wizard-wg-save').addEventListener('click', wizardWgSave);
    document.getElementById('wizard-tor-install').addEventListener('click', wizardTorInstall);
    document.getElementById('wizard-tor-enable').addEventListener('click', wizardTorEnable);

    setupPasswordToggle('wizard-wifi-password', 'wizard-wifi-toggle-pwd');
    setupPasswordToggle('wizard-ap-password', 'wizard-ap-toggle-pwd');

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
