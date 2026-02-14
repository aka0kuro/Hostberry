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
    const btnText = connectBtn && connectBtn.querySelector('.btn-text');
    if (connectBtn) {
      connectBtn.disabled = true;
      if (btnText) btnText.textContent = t('setup_wizard.connecting', 'Conectando...');
    }
    var timeoutId;
    var timeoutMs = 28000;
    var connectPromise = apiRequest('/api/v1/wifi/connect', {
      method: 'POST',
      body: { ssid: selectedSSID, password: password, country: 'ES' }
    });
    var timeoutPromise = new Promise(function(_, reject) {
      timeoutId = setTimeout(function() {
        reject(new Error(t('setup_wizard.error_connect_timeout', 'Tiempo de espera agotado. Comprueba la contraseña e inténtalo de nuevo.')));
      }, timeoutMs);
    });
    try {
      const resp = await Promise.race([connectPromise, timeoutPromise]);
      clearTimeout(timeoutId);
      const data = await resp.json().catch(function() { return {}; });
      if (resp.ok && data.success !== false) {
        showAlert('success', t('setup_wizard.connected', 'Conectado'));
        setStep(2);
      } else {
        showAlert('danger', (data.error || t('setup_wizard.error_connect', 'Error al conectar')));
      }
    } catch (e) {
      clearTimeout(timeoutId);
      showAlert('danger', e && e.message ? e.message : t('setup_wizard.error_connect', 'Error al conectar'));
    } finally {
      if (connectBtn) {
        connectBtn.disabled = false;
        if (btnText) btnText.textContent = t('setup_wizard.connect', 'Conectar');
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

  function init() {
    document.getElementById('wizard-scan-btn').addEventListener('click', scanNetworks);
    document.getElementById('wizard-connect-btn').addEventListener('click', connectWiFi);
    document.getElementById('wizard-back-2').addEventListener('click', function() { setStep(1); });
    document.getElementById('wizard-next-2').addEventListener('click', function() { saveHostapd(); });
    document.getElementById('wizard-back-3').addEventListener('click', function() { setStep(2); });

    document.querySelectorAll('.wizard-security-option').forEach(function(card) {
      card.addEventListener('click', function() {
        selectedSecurityOption = card.getAttribute('data-option');
        document.querySelectorAll('.wizard-security-option').forEach(function(c) { c.classList.remove('border-primary', 'border-3'); });
        card.classList.add('border-primary', 'border-3');
        document.getElementById('wizard-go-config').disabled = false;
      });
    });
    document.getElementById('wizard-go-config').addEventListener('click', function() {
      if (selectedSecurityOption) window.location.href = '/setup-wizard/' + selectedSecurityOption;
    });

    setupPasswordToggle('wizard-wifi-password', 'wizard-wifi-toggle-pwd');
    setupPasswordToggle('wizard-ap-password', 'wizard-ap-toggle-pwd');

    document.getElementById('wizard-ap-open').addEventListener('change', function() {
      document.getElementById('wizard-ap-password-box').classList.toggle('d-none', this.checked);
    });

    var params = new URLSearchParams(window.location.search || '');
    var stepParam = params.get('step');
    if (stepParam === '3') {
      setStep(3);
    } else {
      setStep(1);
    }
    scanNetworks();
  }

  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', init);
  } else {
    init();
  }
})();
