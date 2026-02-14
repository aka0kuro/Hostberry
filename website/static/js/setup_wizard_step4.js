(function () {
  var step = window.WIZARD_STEP4;
  if (!step) return;

  var t = window.HostBerry && window.HostBerry.t ? function (k, d) { return HostBerry.t(k, d); } : function (_, d) { return d || _; };
  var showAlert = window.HostBerry && window.HostBerry.showAlert ? function (type, msg) { HostBerry.showAlert(type, msg); } : function (_, msg) { alert(msg); };
  var apiRequest = window.HostBerry && window.HostBerry.apiRequest ? function (u, o) { return HostBerry.apiRequest(u, o); } : function (u, o) { return fetch(u, Object.assign({ credentials: 'include' }, o)); };

  function initVpn() {
    apiRequest('/api/v1/vpn/config', { method: 'GET' }).then(function (r) { return r.ok ? r.json() : {}; }).then(function (data) {
      var ta = document.getElementById('wizard-openvpn-config');
      if (ta && data && typeof data.config === 'string') ta.value = data.config;
    }).catch(function () {});

    var fileInput = document.getElementById('wizard-openvpn-file');
    if (fileInput) {
      fileInput.addEventListener('change', function () {
        var file = this.files && this.files[0];
        if (!file) return;
        var reader = new FileReader();
        reader.onload = function () { var ta = document.getElementById('wizard-openvpn-config'); if (ta) ta.value = reader.result || ''; };
        reader.readAsText(file);
      });
    }

    var saveBtn = document.getElementById('wizard-openvpn-save');
    var connectBtn = document.getElementById('wizard-openvpn-connect');
    if (saveBtn) {
      saveBtn.addEventListener('click', function () {
        var ta = document.getElementById('wizard-openvpn-config');
        var config = (ta && ta.value) ? ta.value.trim() : '';
        if (!config) { showAlert('warning', t('vpn.config_required', 'Pega o sube una configuración primero')); return; }
        apiRequest('/api/v1/vpn/config', { method: 'POST', body: { config: config } }).then(function (r) { return r.json().catch(function () { return {}; }); }).then(function (d) {
          if (d && !d.error) showAlert('success', d.message || t('common.saved', 'Guardado'));
          else showAlert('danger', d.error || 'Error');
        }).catch(function (e) { showAlert('danger', e.message || 'Error'); });
      });
    }
    if (connectBtn) {
      connectBtn.addEventListener('click', function () {
        var ta = document.getElementById('wizard-openvpn-config');
        var config = (ta && ta.value) ? ta.value.trim() : '';
        if (!config) { showAlert('warning', t('vpn.config_required', 'Pega o sube una configuración primero')); return; }
        apiRequest('/api/v1/vpn/connect', { method: 'POST', body: { config: config, type: 'openvpn' } }).then(function (r) { return r.json().catch(function () { return {}; }); }).then(function (d) {
          if (d && !d.error) { showAlert('success', d.message || t('vpn.connect_vpn', 'Conectar')); }
          else showAlert('danger', d.error || 'Error');
        }).catch(function (e) { showAlert('danger', e.message || 'Error'); });
      });
    }
  }

  function initWireguard() {
    apiRequest('/api/v1/wireguard/config', { method: 'GET' }).then(function (r) { return r.ok ? r.json() : {}; }).then(function (data) {
      var ta = document.getElementById('wizard-wg-config');
      if (ta && data && typeof data.config === 'string') ta.value = data.config;
    }).catch(function () {});

    var fileInput = document.getElementById('wizard-wg-file');
    if (fileInput) {
      fileInput.addEventListener('change', function () {
        var file = this.files && this.files[0];
        if (!file) return;
        var reader = new FileReader();
        reader.onload = function () { var ta = document.getElementById('wizard-wg-config'); if (ta) ta.value = reader.result || ''; };
        reader.readAsText(file);
      });
    }

    var saveBtn = document.getElementById('wizard-wg-save');
    if (saveBtn) {
      saveBtn.addEventListener('click', function () {
        var ta = document.getElementById('wizard-wg-config');
        var config = (ta && ta.value) ? ta.value.trim() : '';
        if (!config) { showAlert('warning', t('setup_wizard.wg_config_empty', 'Escribe o pega la configuración.')); return; }
        apiRequest('/api/v1/wireguard/config', { method: 'POST', body: { config: config } }).then(function (r) { return r.json().catch(function () { return {}; }); }).then(function (d) {
          if (d && !d.error) showAlert('success', t('common.saved', 'Guardado'));
          else showAlert('danger', d.error || 'Error');
        }).catch(function (e) { showAlert('danger', e.message || 'Error'); });
      });
    }
  }

  function loadTorStatus() {
    apiRequest('/api/v1/tor/status', { method: 'GET' }).then(function (r) { return r.ok ? r.json() : {}; }).then(function (s) {
      var dot = document.getElementById('wizard-tor-status-dot');
      var text = document.getElementById('wizard-tor-status-text');
      var installBtn = document.getElementById('wizard-tor-install');
      var enableBtn = document.getElementById('wizard-tor-enable');
      var iptDot = document.getElementById('wizard-tor-iptables-dot');
      var iptLabel = document.getElementById('wizard-tor-iptables-label');
      var iptEnable = document.getElementById('wizard-tor-iptables-enable');
      var iptDisable = document.getElementById('wizard-tor-iptables-disable');
      if (!text) return;
      if (s.installed) {
        if (dot) dot.className = 'status-indicator ' + (s.active ? 'status-online' : 'status-offline');
        text.textContent = s.active ? t('tor.active', 'Activo') : t('tor.inactive', 'Inactivo');
        if (installBtn) installBtn.classList.add('d-none');
        if (enableBtn) { enableBtn.classList.remove('d-none'); enableBtn.textContent = s.active ? t('tor.disable', 'Deshabilitar') : t('tor.enable', 'Habilitar'); }
        if (s.active && iptDot && iptLabel && iptEnable && iptDisable) {
          var iptActive = !!s.iptables_active;
          iptDot.className = 'status-indicator ' + (iptActive ? 'status-online' : 'status-offline');
          iptLabel.textContent = iptActive ? t('tor.torify_active', 'Activo') : t('tor.torify_inactive', 'Inactivo');
          iptEnable.classList.remove('d-none');
          iptDisable.classList.remove('d-none');
          iptEnable.style.display = iptActive ? 'none' : 'inline-block';
          iptDisable.style.display = iptActive ? 'inline-block' : 'none';
        }
      } else {
        if (dot) dot.className = 'status-indicator status-offline';
        text.textContent = t('tor.not_installed', 'No instalado');
        if (installBtn) installBtn.classList.remove('d-none');
        if (enableBtn) enableBtn.classList.add('d-none');
        if (iptLabel) iptLabel.textContent = t('tor.torify_inactive', 'Inactivo');
        if (iptDot) iptDot.className = 'status-indicator status-offline';
        if (iptEnable) iptEnable.classList.add('d-none');
        if (iptDisable) iptDisable.classList.add('d-none');
      }
    }).catch(function () {});
  }

  function initTor() {
    loadTorStatus();

    var installBtn = document.getElementById('wizard-tor-install');
    if (installBtn) {
      installBtn.addEventListener('click', function () {
        var btn = this;
        btn.disabled = true;
        btn.textContent = t('common.loading', 'Cargando...');
        apiRequest('/api/v1/tor/install', { method: 'POST' }).then(function (r) { return r.json().catch(function () { return {}; }); }).then(function (d) {
          if (d && !d.error) { showAlert('success', d.message || t('tor.installed', 'Instalado')); loadTorStatus(); }
          else showAlert('danger', d.error || 'Error');
        }).catch(function (e) { showAlert('danger', e.message || 'Error'); }).finally(function () {
          btn.disabled = false;
          btn.textContent = t('tor.install', 'Instalar Tor');
        });
      });
    }

    var enableBtn = document.getElementById('wizard-tor-enable');
    if (enableBtn) {
      enableBtn.addEventListener('click', function () {
        var btn = this;
        btn.disabled = true;
        apiRequest('/api/v1/tor/enable', { method: 'POST' }).then(function (r) { return r.json().catch(function () { return {}; }); }).then(function (d) {
          if (d && !d.error) { showAlert('success', d.message || t('tor.enabled', 'Habilitado')); loadTorStatus(); }
          else showAlert('danger', d.error || 'Error');
        }).catch(function (e) { showAlert('danger', e.message || 'Error'); }).finally(function () { btn.disabled = false; loadTorStatus(); });
      });
    }

    var iptEn = document.getElementById('wizard-tor-iptables-enable');
    var iptDis = document.getElementById('wizard-tor-iptables-disable');
    if (iptEn) iptEn.addEventListener('click', function () {
      apiRequest('/api/v1/tor/iptables-enable', { method: 'POST' }).then(function (r) { return r.json().catch(function () { return {}; }); }).then(function (d) {
        if (d && !d.error) { showAlert('success', d.message || t('tor.torify_enabled', 'Red torificada')); loadTorStatus(); }
        else showAlert('danger', d.error || 'Error');
      });
    });
    if (iptDis) iptDis.addEventListener('click', function () {
      apiRequest('/api/v1/tor/iptables-disable', { method: 'POST' }).then(function (r) { return r.json().catch(function () { return {}; }); }).then(function (d) {
        if (d && !d.error) { showAlert('success', d.message || t('tor.torify_disabled', 'Redirección desactivada')); loadTorStatus(); }
      });
    });
  }

  if (step === 'vpn') initVpn();
  else if (step === 'wireguard') initWireguard();
  else if (step === 'tor') initTor();
})();
