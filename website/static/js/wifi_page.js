// WiFi Page JavaScript
(function() {
  'use strict';

  // Función de traducción
  const t = (key, defaultValue) => {
    if (window.HostBerry && window.HostBerry.t) {
      return window.HostBerry.t(key, defaultValue);
    }
    return defaultValue || key;
  };

  // Función para traducir mensajes de error del backend
  const translateError = (errorMessage) => {
    if (!errorMessage) return '';
    
    const errorLower = errorMessage.toLowerCase();
    
    // Mapeo de mensajes de error comunes del backend
    const errorMap = {
      'wifi no está habilitado': t('wifi.wifi_not_enabled', 'WiFi is not enabled'),
      'wifi is not enabled': t('wifi.wifi_not_enabled', 'WiFi is not enabled'),
      'por favor, habilita wifi primero': t('wifi.enable_wifi_first', 'Please enable WiFi first'),
      'please enable wifi first': t('wifi.enable_wifi_first', 'Please enable WiFi first'),
      'no se encontraron redes wifi': t('wifi.no_networks_found', 'No WiFi networks found'),
      'no wifi networks found': t('wifi.no_networks_found', 'No WiFi networks found'),
      'verifica que wifi esté habilitado': t('wifi.check_wifi_enabled', 'Make sure WiFi is enabled'),
      'make sure wifi is enabled': t('wifi.check_wifi_enabled', 'Make sure WiFi is enabled'),
      'script no encontrado': t('wifi.script_not_found', 'Script not found'),
      'script not found': t('wifi.script_not_found', 'Script not found'),
      'error conectando a wifi': t('wifi.connect_error', 'Error connecting to WiFi'),
      'error connecting to wifi': t('wifi.connect_error', 'Error connecting to WiFi'),
      'error desconectando de wifi': t('wifi.disconnect_error', 'Error disconnecting from WiFi'),
      'error disconnecting from wifi': t('wifi.disconnect_error', 'Error disconnecting from WiFi'),
      'error cambiando estado de wifi': t('wifi.toggle_error', 'Error toggling WiFi'),
      'error toggling wifi': t('wifi.toggle_error', 'Error toggling WiFi'),
      'error desbloqueando wifi': t('wifi.unblock_error', 'Error unblocking WiFi'),
      'error unblocking wifi': t('wifi.unblock_error', 'Error unblocking WiFi'),
      'error escaneando redes': t('wifi.scan_error', 'Error scanning WiFi networks'),
      'error scanning networks': t('wifi.scan_error', 'Error scanning WiFi networks'),
      'disconnected from': t('wifi.disconnected', 'Disconnected from WiFi'),
      'desconectado de': t('wifi.disconnected', 'Disconnected from WiFi'),
      'connected to': t('wifi.connected_to', 'Connected to'),
      'conectado a': t('wifi.connected_to', 'Connected to'),
      'successfully': t('common.success', 'Successfully'),
      'exitosamente': t('common.success', 'Successfully'),
    };
    
    // Buscar coincidencia exacta o parcial
    for (const [key, translation] of Object.entries(errorMap)) {
      if (errorLower.includes(key)) {
        return translation;
      }
    }
    
    // Si no hay coincidencia, devolver el mensaje original
    return errorMessage;
  };

  const escapeHtmlForDisplay = (text) => {
    if (text == null) return '';
    const s = String(text);
    return (window.HostBerry && window.HostBerry.escapeHtml) ? window.HostBerry.escapeHtml(s) : s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;');
  };

  // Función para mostrar alertas
  const showAlert = (type, message) => {
    if (window.HostBerry && window.HostBerry.showAlert) {
      window.HostBerry.showAlert(type, message);
    } else {
      alert(message);
    }
  };

  // Función para hacer peticiones API
  const apiRequest = async (url, options) => {
    if (window.HostBerry && window.HostBerry.apiRequest) {
      return await window.HostBerry.apiRequest(url, options);
    }
    const opts = Object.assign({ method: 'GET', headers: {} }, options || {});
    return await fetch(url, opts);
  };

  // Variable global para almacenar la red conectada actualmente
  let currentConnectedSSID = null;

  // Cargar estado de conexión
  async function loadConnectionStatus() {
    try {
      const resp = await apiRequest('/api/v1/wifi/status');
      if (!resp || !resp.ok) {
        console.error(t('wifi.status_error', 'Error getting WiFi status') + ':', resp.status);
        return;
      }
      const data = await resp.json();
      // Guardar SSID conectado actualmente
      currentConnectedSSID = (data.connected && data.ssid) ? data.ssid : null;
      updateStatusCards(data);
      updateConnectionInfo(data);
      updateButtonTexts(data);
      // Actualizar botones de conexión en las tarjetas
      updateConnectButtons();
    } catch (error) {
      console.error(t('wifi.status_error', 'Error getting WiFi status') + ':', error);
    }
  }

  // Actualizar tarjetas de estado
  function updateStatusCards(data) {
    const wifiStatusValue = document.getElementById('wifi-status-value');
    const wifiStatusBar = document.getElementById('wifi-status-bar');
    const wifiStatusIcon = document.getElementById('wifi-status-icon');
    const connectionStatusValue = document.getElementById('connection-status-value');
    const signalValue = document.getElementById('signal-value');
    const signalBar = document.getElementById('signal-bar');
    const networksCountValue = document.getElementById('networks-count-value');
    const networksTable = document.getElementById('networksTable');
    
    // WiFi Status
    if (wifiStatusValue && wifiStatusBar && wifiStatusIcon) {
      const enabled = data.enabled || false;
      wifiStatusValue.textContent = enabled ? t('wifi.enabled', 'Enabled') : t('wifi.disabled', 'Disabled');
      wifiStatusBar.style.width = enabled ? '100%' : '0%';
      wifiStatusIcon.className = enabled ? 'bi bi-wifi' : 'bi bi-wifi-off';
      wifiStatusIcon.style.color = enabled ? 'var(--hb-primary)' : 'var(--hb-text-muted)';
    }

    // Connection Status
    if (connectionStatusValue) {
      const connected = data.connected || false;
      const ssid = data.ssid || '--';
      const statusText = connected ? ssid : t('wifi.disconnected', 'Disconnected');
      connectionStatusValue.textContent = statusText;
      // Agregar clase para texto desconectado
      if (!connected) {
        connectionStatusValue.classList.add('text-disconnected');
        connectionStatusValue.setAttribute('data-status', 'disconnected');
      } else {
        connectionStatusValue.classList.remove('text-disconnected');
        connectionStatusValue.removeAttribute('data-status');
      }
    }

    // Signal
    if (signalValue && signalBar) {
      // Buscar signal en connection_info primero, luego en data directamente
      let signal = null;
      if (data.connection_info && data.connection_info.signal) {
        const signalStr = String(data.connection_info.signal).trim();
        if (signalStr && signalStr !== "null" && signalStr !== "0" && signalStr !== "") {
          const signalNum = parseInt(signalStr, 10);
          if (!isNaN(signalNum) && signalNum !== 0) {
            signal = signalNum;
          }
        }
      } else if (data.signal) {
        const signalStr = String(data.signal).trim();
        if (signalStr && signalStr !== "null" && signalStr !== "0" && signalStr !== "") {
          const signalNum = parseInt(signalStr, 10);
          if (!isNaN(signalNum) && signalNum !== 0) {
            signal = signalNum;
          }
        }
      }
      // Si la señal es positiva, convertir a negativa (error de parsing)
      if (signal !== null && signal > 0) {
        signal = -signal;
      }
      // Calcular porcentaje: -30 dBm = 100%, -100 dBm = 0%
      let signalPercent = 0;
      if (signal !== null && signal !== 0 && !isNaN(signal)) {
        const signalPercentRaw = signal <= -100 ? 0 : (signal >= -30 ? 100 : Math.min(100, Math.max(0, ((signal + 100) / 70) * 100)));
        signalPercent = Math.round(signalPercentRaw); // Redondear a número entero
      }
      if (signalValue) {
        if (signal !== null && signal !== 0 && !isNaN(signal) && signal < 0) {
          signalValue.textContent = signal + 'dBm';
        } else {
          signalValue.textContent = '--';
        }
      }
      signalBar.style.width = signalPercent + '%';
    }

    // Networks Count
    if (networksCountValue && networksTable) {
      const count = networksTable.querySelectorAll('.network-card').length;
      networksCountValue.textContent = count;
    }
  }

  // Actualizar información de conexión
  function updateConnectionInfo(data) {
      const statusEl = document.getElementById('connection-status');
      const ssidEl = document.getElementById('connection-ssid');
      const signalEl = document.getElementById('connection-signal');
      const securityEl = document.getElementById('connection-security');
      const channelEl = document.getElementById('connection-channel');
      const ipEl = document.getElementById('connection-ip');
      const macEl = document.getElementById('connection-mac');
      const speedEl = document.getElementById('connection-speed');
      
    if (statusEl) {
      const connected = data.connected || false;
      const statusText = connected 
        ? t('wifi.connected', 'Connected') 
        : t('wifi.disconnected', 'Disconnected');
      statusEl.textContent = statusText;
      // Agregar clase para texto desconectado
      if (!connected) {
        statusEl.classList.add('text-disconnected');
        statusEl.setAttribute('data-status', 'disconnected');
      } else {
        statusEl.classList.remove('text-disconnected');
        statusEl.removeAttribute('data-status');
      }
    }
    if (ssidEl) ssidEl.textContent = data.ssid || '--';
    
    // Buscar signal en connection_info primero
    let signal = null;
    if (data.connection_info && data.connection_info.signal !== null && data.connection_info.signal !== undefined) {
      const signalValue = data.connection_info.signal;
      // Verificar que no sea null, undefined, o la cadena "null"
      if (signalValue !== null && signalValue !== undefined && signalValue !== "null") {
        const signalStr = String(signalValue).trim();
        if (signalStr && signalStr !== "null" && signalStr !== "0" && signalStr !== "" && signalStr !== "undefined") {
          const signalNum = parseInt(signalStr, 10);
          if (!isNaN(signalNum) && signalNum !== 0) {
            signal = signalNum;
          }
        }
      }
    } else if (data.signal !== null && data.signal !== undefined && data.signal !== "null") {
      const signalValue = data.signal;
      if (signalValue !== null && signalValue !== undefined) {
        const signalStr = String(signalValue).trim();
        if (signalStr && signalStr !== "null" && signalStr !== "0" && signalStr !== "" && signalStr !== "undefined") {
          const signalNum = parseInt(signalStr, 10);
          if (!isNaN(signalNum) && signalNum !== 0) {
            signal = signalNum;
          }
        }
      }
    }
    if (signalEl) {
      if (signal !== null && signal !== undefined && signal !== 0 && !isNaN(signal)) {
        signalEl.textContent = signal + 'dBm';
      } else {
        signalEl.textContent = '--';
      }
    }
    
    // Buscar security en connection_info primero
    const security = (data.connection_info && data.connection_info.security) || data.security || '--';
    if (securityEl) securityEl.textContent = security;
    
    // Buscar channel en connection_info primero
    let channel = '--';
    if (data.connection_info && data.connection_info.channel !== null && data.connection_info.channel !== undefined) {
      const channelValue = data.connection_info.channel;
      if (channelValue !== null && channelValue !== undefined && channelValue !== "null" && channelValue !== "") {
        channel = String(channelValue).trim();
        if (channel === "null" || channel === "undefined" || channel === "") {
          channel = '--';
        }
      }
    } else if (data.channel !== null && data.channel !== undefined && data.channel !== "null" && data.channel !== "") {
      channel = String(data.channel).trim();
      if (channel === "null" || channel === "undefined" || channel === "") {
        channel = '--';
      }
    }
    if (channelEl) channelEl.textContent = channel;
    
    // Buscar ip en connection_info primero
    const ip = (data.connection_info && data.connection_info.ip) || data.ip || '--';
    if (ipEl) {
      if (ip === '--' || ip === '' || ip === 'N/A') {
        // Si no hay IP pero está "conectado", mostrar "Obtaining IP..."
        if (data.connected && data.ssid) {
          ipEl.textContent = t('wifi.obtaining_ip', 'Obtaining IP...');
        } else {
          ipEl.textContent = '--';
        }
      } else {
        ipEl.textContent = ip;
      }
    }
    
    // Buscar mac en connection_info primero
    const mac = (data.connection_info && data.connection_info.mac) || data.mac || '--';
    if (macEl) macEl.textContent = mac;
    
    // Buscar speed en connection_info primero
    const speed = (data.connection_info && data.connection_info.speed) || data.speed || '--';
    if (speedEl) speedEl.textContent = speed;
  }

  // Cargar interfaces WiFi
  async function loadInterfaces() {
    try {
      const resp = await apiRequest('/api/v1/wifi/interfaces');
      if (!resp || !resp.ok) {
        console.warn('Error obteniendo interfaces WiFi:', resp.status);
        return;
      }
      const data = await resp.json();
      const select = document.getElementById('wifi-interface');
      if (!select) return;
      
      select.innerHTML = '<option value="">' + t('wifi.auto_detect', 'Auto-detect') + '</option>';
      
      // El backend devuelve {success: true, interfaces: [...]}
      // donde interfaces es un array de objetos {name, type, state}
      let interfaces = [];
      if (data.interfaces && Array.isArray(data.interfaces)) {
        interfaces = data.interfaces;
      } else if (Array.isArray(data)) {
        interfaces = data;
      }
      
      if (interfaces.length === 0) {
        console.warn('No se encontraron interfaces WiFi');
      }
      
      interfaces.forEach(iface => {
        // Si es un objeto, extraer el nombre
        let ifaceName = '';
        if (typeof iface === 'object' && iface !== null) {
          ifaceName = iface.name || iface.interface || iface.device || '';
        } else if (typeof iface === 'string') {
          ifaceName = iface;
        }
        
        if (ifaceName && ifaceName !== '' && ifaceName !== '--') {
          const option = document.createElement('option');
          option.value = ifaceName;
          option.textContent = ifaceName;
          select.appendChild(option);
        }
      });
    } catch (error) {
      console.error(t('wifi.networks_error', 'Error getting WiFi networks') + ':', error);
    }
  }
  
  // Toggle WiFi
  async function toggleWiFi() {
    const btn = document.getElementById('toggle-wifi-btn');
    const icon = document.getElementById('toggle-wifi-icon');
    const text = document.getElementById('toggle-wifi-text');
    
    if (!btn || btn.disabled) return;
    
      btn.disabled = true;
    const originalText = text.textContent;
    text.textContent = t('wifi.enabling', 'Enabling...');
      
      try {
        const resp = await apiRequest('/api/v1/wifi/toggle', { method: 'POST' });
      const data = await resp.json();
      
      if (resp.ok && data.success) {
        showAlert('success', t('wifi.wifi_toggled', 'WiFi state changed successfully'));
        // Actualizar texto del botón según el nuevo estado
        const statusResp = await apiRequest('/api/v1/wifi/status');
        if (statusResp.ok) {
          const statusData = await statusResp.json();
          if (text) {
            text.textContent = statusData.enabled 
              ? t('wifi.disable_wifi', 'Disable WiFi')
              : t('wifi.enable_wifi', 'Enable WiFi');
          }
          if (icon) {
            icon.className = statusData.enabled ? 'bi bi-wifi-off' : 'bi bi-wifi';
          }
        }
          await loadConnectionStatus();
          } else {
        showAlert('danger', translateError(data.error) || t('wifi.toggle_error', 'Error toggling WiFi'));
      }
    } catch (error) {
      console.error(t('wifi.toggle_error', 'Error toggling WiFi') + ':', error);
      showAlert('danger', t('wifi.toggle_error', 'Error toggling WiFi'));
      } finally {
          btn.disabled = false;
      text.textContent = originalText;
    }
  }
  
  // Unblock WiFi
  async function unblockWiFi() {
    const btn = document.getElementById('unblock-wifi-btn');
    if (!btn || btn.disabled) return;
    
      btn.disabled = true;
    const originalText = btn.querySelector('span').textContent;
    btn.querySelector('span').textContent = t('wifi.unblocking', 'Unblocking...');
      
      try {
        const resp = await apiRequest('/api/v1/wifi/unblock', { method: 'POST' });
      const data = await resp.json();
      
      if (resp.ok && data.success) {
          showAlert('success', t('wifi.wifi_unblocked', 'WiFi unblocked successfully'));
        btn.style.display = 'none';
            await loadConnectionStatus();
        } else {
        showAlert('danger', translateError(data.error) || t('wifi.unblock_error', 'Error unblocking WiFi'));
      }
    } catch (error) {
      console.error(t('wifi.unblock_error', 'Error unblocking WiFi') + ':', error);
      showAlert('danger', t('wifi.unblock_error', 'Error unblocking WiFi'));
      } finally {
          btn.disabled = false;
      btn.querySelector('span').textContent = originalText;
    }
  }

  // Toggle Software Switch
  async function toggleSoftwareSwitch() {
    const btn = document.getElementById('toggle-software-switch-btn');
    const icon = document.getElementById('software-switch-icon');
    const text = document.getElementById('software-switch-text');
    
    if (!btn || btn.disabled) return;
    
    btn.disabled = true;
    
    try {
      const resp = await apiRequest('/api/v1/wifi/software-switch', { method: 'POST' });
        const data = await resp.json();
      
      if (resp.ok && data.success) {
        showAlert('success', t('wifi.software_switch_toggled', 'Software switch toggled successfully'));
        if (icon) {
          icon.className = data.enabled ? 'bi bi-toggle-on' : 'bi bi-toggle-off';
        }
        if (text) {
          text.textContent = data.enabled 
            ? t('wifi.disable_software_switch', 'Disable Software Switch')
            : t('wifi.enable_software_switch', 'Enable Software Switch');
        }
        await loadConnectionStatus();
            } else {
        showAlert('danger', translateError(data.error) || t('wifi.toggle_error', 'Error toggling switch'));
      }
    } catch (error) {
      console.error(t('wifi.toggle_error', 'Error toggling switch') + ':', error);
      showAlert('danger', t('wifi.toggle_error', 'Error toggling switch'));
    } finally {
      btn.disabled = false;
    }
  }

  // Escanear redes
  async function scanNetworks() {
    const loadingEl = document.getElementById('networks-loading');
    const emptyEl = document.getElementById('networks-empty');
    const tableEl = document.getElementById('networks-table-container');
    const tbody = document.getElementById('networksTable');
    const interfaceSelect = document.getElementById('wifi-interface');

    // Asegurar que el software switch y el WiFi estén activos antes de escanear
    const ensureWifiReady = async () => {
      try {
        const statusResp = await apiRequest('/api/v1/wifi/status');
        if (!statusResp.ok) return;
        const statusData = await statusResp.json();

        if (statusData.soft_blocked || !statusData.enabled) {
          showAlert('info', t('wifi.activating', 'Activating WiFi...'));
        }

        if (statusData.soft_blocked) {
          await apiRequest('/api/v1/wifi/software-switch', { method: 'POST' });
        }
        if (!statusData.enabled) {
          await apiRequest('/api/v1/wifi/toggle', { method: 'POST' });
        }

        // Esperar un poco para que el sistema aplique el cambio
        await new Promise(resolve => setTimeout(resolve, 1500));

        // Forzar refresco del estado después de activar
        await loadConnectionStatus();
      } catch (error) {
        console.error(t('wifi.toggle_error', 'Error toggling WiFi') + ':', error);
      }
    };

    await ensureWifiReady();
    
    if (loadingEl) loadingEl.style.display = 'block';
    if (emptyEl) emptyEl.style.display = 'none';
    if (tableEl) tableEl.style.display = 'none';
    if (tbody) tbody.innerHTML = '';
    
    try {
      let url = '/api/v1/wifi/scan';
      
      // Verificar token antes de escanear
      const token = localStorage.getItem('access_token');
      if (!token) {
        showAlert('warning', t('auth.session_expired', 'Session expired. Please log in again.'));
        setTimeout(() => {
          window.location.href = '/login';
        }, 2000);
        return;
      }
      const selectedInterface = interfaceSelect ? interfaceSelect.value : '';
      if (selectedInterface) {
        url += '?interface=' + encodeURIComponent(selectedInterface);
      }
      
      const resp = await apiRequest(url);
      
      if (!resp || !resp.ok) {
        let errorText = '';
        try {
          errorText = await resp.text();
        } catch (e) {
          console.error(t('common.error', 'Error') + ':', e);
        }
        
        if (loadingEl) loadingEl.style.display = 'none';
        
        if (emptyEl) {
          emptyEl.style.display = 'block';
          emptyEl.innerHTML = `
            <div class="text-center py-5">
              <i class="bi bi-exclamation-triangle text-warning" style="font-size: 4rem;"></i>
              <p class="mt-3">${t('errors.scan_failed', 'Scan failed')}</p>
              <p class="text-muted small">${escapeHtmlForDisplay(translateError(errorText) || t('errors.unknown_error', 'Unknown error'))}</p>
              <button class="btn btn-primary mt-3" onclick="scanNetworks()">
                <i class="bi bi-arrow-clockwise me-2"></i>${t('common.retry', 'Retry')}
              </button>
            </div>
          `;
        }
        // Manejar error 401 (sesión expirada o usuario no encontrado)
        if (resp.status === 401) {
          // Intentar leer el mensaje de error para determinar si es un error de usuario no encontrado
          let errorData = null;
          try {
            const text = await resp.text();
            if (text) {
              errorData = JSON.parse(text);
            }
          } catch (e) {
            // Ignorar errores de parsing
          }
          
          // Si es "Usuario no encontrado", puede ser un problema temporal de la base de datos
          if (errorData && errorData.error && (
            errorData.error.includes('Usuario no encontrado') || 
            errorData.error.includes('User not found') ||
            errorData.code === 'USER_NOT_FOUND'
          )) {
            showAlert('warning', t('wifi.scan_error', 'Error scanning WiFi networks') + ': ' + (errorData.error || 'Usuario no encontrado. Por favor, recarga la página e intenta nuevamente.'));
            if (loadingEl) loadingEl.style.display = 'none';
            if (emptyEl) {
              emptyEl.style.display = 'block';
              emptyEl.innerHTML = `
                <div class="text-center py-5">
                  <i class="bi bi-exclamation-triangle text-warning" style="font-size: 4rem;"></i>
                  <p class="mt-3">${t('wifi.scan_error', 'Error scanning WiFi networks')}</p>
                  <p class="text-muted small">${escapeHtmlForDisplay(errorData.error || 'Usuario no encontrado')}</p>
                  <button class="btn btn-primary mt-3" onclick="location.reload()">
                    <i class="bi bi-arrow-clockwise me-2"></i>Recargar página
                  </button>
                </div>`;
            }
            return;
          }
          
          // Para otros errores 401, cerrar sesión normalmente
          showAlert('warning', t('auth.session_expired', 'Session expired. Please log in again.'));
          setTimeout(() => {
            localStorage.removeItem('access_token');
            window.location.href = '/login?error=session_expired';
          }, 2000);
          return;
        }
        throw new Error(t('wifi.scan_error', 'Error scanning WiFi networks') + ': HTTP ' + resp.status);
      }
      
      let data;
      try {
        const text = await resp.text();
        if (!text || text.trim() === '') {
          data = { networks: [] };
        } else {
          data = JSON.parse(text);
        }
      } catch (parseError) {
        console.error(t('common.error', 'Error') + ' parsing JSON:', parseError);
        data = { networks: [] };
      }
      
      if (loadingEl) loadingEl.style.display = 'none';
      
      let networks = [];
      if (data.networks && Array.isArray(data.networks)) {
        networks = data.networks;
      } else if (data && Array.isArray(data)) {
        networks = data;
      }
      
      // Filtrar redes inválidas (mensajes de error, etc.)
      networks = networks.filter(net => {
        const ssid = (net.ssid || '').trim();
        const ssidLower = ssid.toLowerCase();
        
        // Filtrar redes con nombres inválidos o mensajes de error
        if (!ssid || ssid === '' || ssid === '--' || 
            ssidLower === 'sudo' || 
            ssidLower.includes('read-only') || 
            ssidLower.includes('read only') ||
            ssidLower.includes('file system') ||
            ssidLower.includes('filesystem') ||
            ssidLower.includes('error') ||
            ssidLower.includes('permission') ||
            ssidLower.includes('denied') ||
            ssidLower.includes('unable to') ||
            ssidLower.includes('cannot') ||
            ssidLower.includes('failed') ||
            ssidLower.startsWith('sudo:') ||
            ssidLower.includes('sudo ')) {
          return false;
        }
        
        // Filtrar si el signal es 0 o inválido y el SSID parece ser un error
        const signal = parseInt(net.signal || net.rssi || 0);
        if (signal === 0 && (ssid.length > 50 || ssid.includes(':') || ssid.includes('sudo'))) {
          return false;
        }
        
        // Filtrar SSIDs que son claramente mensajes de error del sistema
        if (ssid.match(/^(sudo|error|permission|denied|failed|cannot|unable)/i)) {
          return false;
        }
        
        return true;
      });
      
      if (networks.length === 0) {
        if (emptyEl) emptyEl.style.display = 'block';
        if (tableEl) tableEl.style.display = 'none';
      } else {
        if (emptyEl) emptyEl.style.display = 'none';
        if (tableEl) tableEl.style.display = 'block';
        if (tbody) {
          tbody.innerHTML = '';
          networks.forEach((net, index) => {
            // La señal viene en dBm (negativa, típicamente entre -30 y -100)
            let signal = parseInt(net.signal || net.rssi || 0);
            
            // Si la señal es 0, puede ser un error de parsing - no mostrar porcentaje
            if (signal === 0 || signal === null || signal === undefined) {
              signal = null; // No mostrar señal si es 0 o inválida
            } else {
              // Si la señal es positiva, puede ser un error de parsing
              if (signal > 0) {
                signal = -signal; // Convertir a negativo si es positivo
              }
              // Asegurar que esté en el rango válido
              if (signal > -30) signal = -30; // Límite superior
              if (signal < -100) signal = -100; // Límite inferior
            }
            
            // Calcular porcentaje solo si hay señal válida
            let signalPercent = 0;
            if (signal !== null && signal !== 0) {
              // Calcular porcentaje: -30 dBm = 100%, -100 dBm = 0%
              // Fórmula: ((signal + 100) / 70) * 100, limitado entre 0 y 100
              const signalPercentRaw = signal <= -100 ? 0 : (signal >= -30 ? 100 : Math.min(100, Math.max(0, ((signal + 100) / 70) * 100)));
              signalPercent = Math.round(signalPercentRaw); // Redondear a número entero
            }
            const signalColor = signalPercent > 70 ? 'text-success' : (signalPercent > 40 ? 'text-warning' : 'text-danger');
            const security = net.security || net.encryption || 'none';
            const securityIcon = security === 'none' || security === 'Open' ? 'bi-unlock' : 'bi-lock';
            const ssid = net.ssid || t('wifi.unnamed_network', 'Unnamed network') + ' ' + (index + 1);
            const displaySsid = escapeHtmlForDisplay(ssid);
            const displaySecurity = escapeHtmlForDisplay(String(security || '').toUpperCase());
            const displayChannel = net.channel ? escapeHtmlForDisplay(net.channel) : '';
            
            const card = document.createElement('div');
            card.className = 'network-card';
            // Guardar información en atributos data para acceso fácil
            card.setAttribute('data-ssid', ssid);
            card.setAttribute('data-security', security);
            card.setAttribute('data-signal', signal);
            card.setAttribute('data-channel', net.channel || '');
            
            // Verificar si esta es la red conectada actualmente
            const isConnected = currentConnectedSSID && ssid === currentConnectedSSID;
            const buttonClass = isConnected ? 'btn-success' : 'btn-primary';
            const buttonIcon = isConnected ? 'bi-x-circle' : 'bi-box-arrow-in-right';
            const buttonText = isConnected ? t('wifi.disconnect', 'Disconnect') : t('wifi.connect', 'Connect');
            
            // Traducción para "Connected" badge
            const connectedBadge = isConnected ? ` <span class="badge bg-success ms-2">${t('wifi.connected', 'Connected')}</span>` : '';
            
            card.innerHTML = `
              <div class="network-card-content">
                <div class="network-card-icon ${signalColor}">
                  <i class="bi bi-wifi"></i>
                </div>
                <div class="network-card-info">
                  <div class="network-card-ssid">${displaySsid}${connectedBadge}</div>
                  <div class="network-card-details">
                    <span class="network-card-detail-item">
                      <i class="bi bi-bar-chart me-1"></i> ${signal !== null && signal !== 0 ? signal + 'dBm (' + signalPercent + '%)' : '--'}
                    </span>
                    <span class="network-card-detail-item">
                      <i class="bi ${securityIcon} me-1"></i> ${displaySecurity}
                    </span>
                    ${net.channel ? `<span class="network-card-detail-item"><i class="bi bi-hash me-1"></i> ${displayChannel}</span>` : ''}
                  </div>
                </div>
              </div>
              <div class="network-card-actions">
                <button class="btn ${buttonClass} btn-sm" type="button">
                  <i class="bi ${buttonIcon} me-2"></i>${buttonText}
                </button>
              </div>
            `;
            const actionButton = card.querySelector('.network-card-actions button');
            if (actionButton) {
              actionButton.onclick = () => {
                if (isConnected) {
                  disconnectWiFi(actionButton);
                } else {
                  connectToNetwork(ssid, security, actionButton);
                }
              };
            }
            tbody.appendChild(card);
          });
        }
      }
      
      updateStatusCards({});
      await loadConnectionStatus();
    } catch (error) {
      console.error(t('wifi.scan_error', 'Error scanning WiFi networks') + ':', error);
      
      if (loadingEl) loadingEl.style.display = 'none';
      
      if (emptyEl) {
        emptyEl.style.display = 'block';
        emptyEl.innerHTML = `
          <div class="text-center py-5">
            <i class="bi bi-exclamation-triangle text-danger" style="font-size: 4rem;"></i>
            <p class="mt-3">${t('errors.scan_error', 'Scan error')}</p>
            <p class="text-muted small">${escapeHtmlForDisplay(translateError(error.message))}</p>
            <button class="btn btn-primary mt-3" onclick="scanNetworks()">
              <i class="bi bi-arrow-clockwise me-2"></i>${t('common.retry', 'Retry')}
            </button>
          </div>
        `;
      }
    }
  }
  
  // Conectar a red
  async function connectToNetwork(ssid, security, buttonElement) {
    const card = buttonElement ? buttonElement.closest('.network-card') : null;
    let formWrapper = card ? card.querySelector('.network-connect-form-wrapper') : null;
    
    // Si ya hay un formulario, removerlo
    if (formWrapper) {
        formWrapper.remove();
          return;
        }
        
    // Obtener información de la red desde los atributos data de la tarjeta
    let signal = '--';
    let channel = '--';
    let encryption = security || t('wifi.unknown', 'Unknown');
    
    if (card) {
      const cardSignal = card.getAttribute('data-signal');
      const cardChannel = card.getAttribute('data-channel');
      const cardSecurity = card.getAttribute('data-security');
      
      if (cardSignal && cardSignal !== '0' && cardSignal !== '') {
        signal = cardSignal + 'dBm';
      }
      if (cardChannel && cardChannel !== '') {
        channel = cardChannel;
      }
      if (cardSecurity && cardSecurity !== '') {
        encryption = cardSecurity;
      }
    }
    
    // Crear formulario
    formWrapper = document.createElement('div');
    formWrapper.className = 'network-connect-form-wrapper';
    
    const needsPassword = security && security !== 'none' && security !== 'Open' && security !== 'open';
    const safeSignal = escapeHtmlForDisplay(signal);
    const safeEncryption = escapeHtmlForDisplay(String(encryption || '').toUpperCase());
    const safeChannel = escapeHtmlForDisplay(channel !== '--' ? channel : '--');
    const passwordInputId = 'network-password-' + String(ssid).replace(/[^a-zA-Z0-9]/g, '-');
    
    formWrapper.innerHTML = `
      <div class="network-connect-form show">
        <div class="network-connect-info mb-3">
          <div class="row g-2">
            <div class="col-4">
              <div class="network-info-item">
                <label class="network-info-label">${t('wifi.network_signal', 'Signal')}</label>
                <div class="network-info-value">${safeSignal}</div>
              </div>
            </div>
            <div class="col-4">
              <div class="network-info-item">
                <label class="network-info-label">${t('wifi.network_security', 'Security')}</label>
                <div class="network-info-value">${safeEncryption}</div>
              </div>
            </div>
            <div class="col-4">
              <div class="network-info-item">
                <label class="network-info-label">${t('wifi.network_channel', 'Channel')}</label>
                <div class="network-info-value">${safeChannel}</div>
              </div>
            </div>
          </div>
        </div>
        <div class="network-connect-form-group">
          <label class="network-connect-form-label">${t('wifi.network_password', 'Password')}</label>
          <div class="password-input-wrapper">
            <input type="password" class="network-connect-form-input" id="${passwordInputId}" 
                   placeholder="${needsPassword ? t('wifi.network_password', 'Password') : t('wifi.open_network', 'Open network')}" 
                   ${needsPassword ? 'required' : ''}>
            <button type="button" class="password-toggle-btn" title="${t('common.show_password', 'Show password')}">
              <i class="bi bi-eye"></i>
            </button>
          </div>
        </div>
        <div class="network-connect-form-actions">
          <button type="button" class="btn btn-secondary btn-sm network-connect-cancel">
            ${t('common.cancel', 'Cancel')}
          </button>
          <button type="button" class="btn btn-primary btn-sm network-connect-submit">
            <i class="bi bi-box-arrow-in-right me-2"></i>${t('wifi.connect', 'Connect')}
          </button>
        </div>
      </div>
    `;
    const togglePasswordBtn = formWrapper.querySelector('.password-toggle-btn');
    if (togglePasswordBtn) {
      togglePasswordBtn.addEventListener('click', () => togglePasswordVisibility(togglePasswordBtn));
    }
    const cancelBtn = formWrapper.querySelector('.network-connect-cancel');
    if (cancelBtn) {
      cancelBtn.addEventListener('click', () => formWrapper.remove());
    }
    const submitBtn = formWrapper.querySelector('.network-connect-submit');
    if (submitBtn) {
      submitBtn.addEventListener('click', () => submitConnect(ssid, security, submitBtn));
    }
    
    if (card) {
      card.appendChild(formWrapper);
      } else {
      // Si no hay card, mostrar modal o alert
      const password = needsPassword ? prompt(t('wifi.network_password', 'Password') + ':', '') : '';
      if (password !== null) {
        await submitConnectDirect(ssid, security, password);
      }
    }
  }

  // Toggle password visibility
  function togglePasswordVisibility(button) {
    if (!button) {
      console.warn("togglePasswordVisibility: button is null");
      return;
    }
    
    // Buscar el input dentro del mismo wrapper
    const wrapper = button.closest('.password-input-wrapper');
    if (!wrapper) {
      console.warn("togglePasswordVisibility: password-input-wrapper not found");
      return;
    }
    
    // Buscar el input dentro del wrapper
    let input = wrapper.querySelector('input[type="password"]');
    if (!input) {
      input = wrapper.querySelector('input[type="text"]');
    }
    // Si aún no se encuentra, buscar cualquier input
    if (!input) {
      input = wrapper.querySelector('input');
    }
    
    if (!input) {
      console.warn("togglePasswordVisibility: input not found in wrapper", wrapper);
      return;
    }
    
    // Obtener el icono dentro del botón
    const icon = button.querySelector('i');
    
    // Cambiar el tipo de input y el icono
    if (input.type === 'password') {
      input.type = 'text';
      if (icon) {
        icon.className = 'bi bi-eye-slash';
      } else {
        button.innerHTML = '<i class="bi bi-eye-slash"></i>';
      }
      button.setAttribute('title', t('common.hide_password', 'Hide password'));
    } else if (input.type === 'text') {
      input.type = 'password';
      if (icon) {
        icon.className = 'bi bi-eye';
      } else {
        button.innerHTML = '<i class="bi bi-eye"></i>';
      }
      button.setAttribute('title', t('common.show_password', 'Show password'));
    }
  }
  
  // Enviar conexión
  async function submitConnect(ssid, security, buttonElement) {
    const formWrapper = buttonElement.closest('.network-connect-form-wrapper');
    const passwordInput = formWrapper.querySelector('input[type="password"], input[type="text"]');
    const password = passwordInput ? passwordInput.value : '';
    
    buttonElement.disabled = true;
    buttonElement.innerHTML = `<i class="bi bi-arrow-clockwise spinning me-2"></i>${t('wifi.connecting', 'Connecting...')}`;
      
      try {
        // Obtener país desde el selector
        const countrySelect = document.getElementById('wifi-country');
        const country = countrySelect ? countrySelect.value || 'US' : 'US';
        
        const resp = await apiRequest('/api/v1/wifi/connect', {
          method: 'POST',
          body: { 
            ssid: ssid, 
          password: password,
          security: security,
          country: country
        }
      });
        
        // Si la respuesta es 401, puede ser que se perdió la conexión durante el proceso
        if (resp.status === 401) {
          showAlert('warning', t('wifi.connect_session_lost', 'Session lost during connection. This may happen if the network connection was interrupted. Please log in again and check if the WiFi connection was successful.'));
          // Esperar un momento antes de redirigir para dar tiempo a que el usuario vea el mensaje
          setTimeout(() => {
            localStorage.removeItem('access_token');
            window.location.href = '/login?error=session_expired';
          }, 3000);
          buttonElement.disabled = false;
          buttonElement.innerHTML = `<i class="bi bi-box-arrow-in-right me-2"></i>${t('wifi.connect', 'Connect')}`;
          return;
        }
        
        const data = await resp.json();
        
      if (resp.ok && data.success) {
        showAlert('success', t('wifi.connected_to', 'Connected to {ssid}').replace('{ssid}', ssid));
            formWrapper.remove();
        await loadConnectionStatus();
        // Opcional: escanear nuevamente después de conectar
        setTimeout(() => scanNetworks(), 2000);
        } else {
        showAlert('danger', translateError(data.error) || t('wifi.connect_error', 'Error connecting to WiFi'));
        buttonElement.disabled = false;
        buttonElement.innerHTML = `<i class="bi bi-box-arrow-in-right me-2"></i>${t('wifi.connect', 'Connect')}`;
      }
    } catch (error) {
      console.error(t('wifi.connect_error', 'Error connecting to WiFi') + ':', error);
      
      // Verificar si es un error de red (posible pérdida de conexión temporal)
      const isNetworkError = error.message && (
        error.message.includes('Failed to fetch') ||
        error.message.includes('NetworkError') ||
        error.message.includes('network') ||
        error.message.includes('ERR_INTERNET_DISCONNECTED') ||
        error.message.includes('ERR_NETWORK_CHANGED')
      );
      
      if (isNetworkError) {
        showAlert('warning', t('wifi.connect_network_error', 'Network connection lost during WiFi setup. Please check your connection and try again.'));
      } else {
        showAlert('danger', translateError(error.message) || t('wifi.connect_error', 'Error connecting to WiFi'));
      }
      
      buttonElement.disabled = false;
      buttonElement.innerHTML = `<i class="bi bi-box-arrow-in-right me-2"></i>${t('wifi.connect', 'Connect')}`;
    }
  }

  // Enviar conexión directa (sin formulario)
  async function submitConnectDirect(ssid, security, password) {
    try {
      // Obtener país desde el selector
      const countrySelect = document.getElementById('wifi-country');
      const country = countrySelect ? countrySelect.value || 'US' : 'US';
      
      const resp = await apiRequest('/api/v1/wifi/connect', {
        method: 'POST',
        body: {
          ssid: ssid,
          password: password || '',
          security: security,
          country: country
        }
      });
      
      // Si la respuesta es 401, puede ser que se perdió la conexión durante el proceso
      if (resp.status === 401) {
        showAlert('warning', t('wifi.connect_session_lost', 'Session lost during connection. This may happen if the network connection was interrupted. Please log in again and check if the WiFi connection was successful.'));
        setTimeout(() => {
          localStorage.removeItem('access_token');
          window.location.href = '/login?error=session_expired';
        }, 3000);
        return;
      }
      
      const data = await resp.json();
      
      if (resp.ok && data.success) {
        showAlert('success', t('wifi.connected_to', 'Connected to {ssid}').replace('{ssid}', ssid));
        await loadConnectionStatus();
        setTimeout(() => scanNetworks(), 2000);
      } else {
        showAlert('danger', translateError(data.error) || t('wifi.connect_error', 'Error connecting to WiFi'));
      }
    } catch (error) {
      console.error(t('wifi.connect_error', 'Error connecting to WiFi') + ':', error);
      
      // Verificar si es un error de red (posible pérdida de conexión temporal)
      const isNetworkError = error.message && (
        error.message.includes('Failed to fetch') ||
        error.message.includes('NetworkError') ||
        error.message.includes('network') ||
        error.message.includes('ERR_INTERNET_DISCONNECTED') ||
        error.message.includes('ERR_NETWORK_CHANGED')
      );
      
      if (isNetworkError) {
        showAlert('warning', t('wifi.connect_network_error', 'Network connection lost during WiFi setup. Please check your connection and try again.'));
      } else {
        showAlert('danger', translateError(error.message) || t('wifi.connect_error', 'Error connecting to WiFi'));
      }
    }
  }

  // Actualizar botones de conexión en las tarjetas
  function updateConnectButtons() {
    const cards = document.querySelectorAll('.network-card');
    cards.forEach(card => {
      const ssid = card.getAttribute('data-ssid');
      const actionsDiv = card.querySelector('.network-card-actions');
      if (!actionsDiv || !ssid) return;
      
      const isConnected = currentConnectedSSID && ssid === currentConnectedSSID;
      const existingBtn = actionsDiv.querySelector('button');
      
      if (existingBtn) {
        const buttonClass = isConnected ? 'btn-success' : 'btn-primary';
        const buttonIcon = isConnected ? 'bi-x-circle' : 'bi-box-arrow-in-right';
        const buttonText = isConnected ? t('wifi.disconnect', 'Disconnect') : t('wifi.connect', 'Connect');
        const security = card.getAttribute('data-security') || 'none';
        
        existingBtn.className = `btn ${buttonClass} btn-sm`;
        existingBtn.innerHTML = `<i class="bi ${buttonIcon} me-2"></i>${buttonText}`;
        existingBtn.onclick = () => {
          if (isConnected) {
            disconnectWiFi(existingBtn);
          } else {
            connectToNetwork(ssid, security, existingBtn);
          }
        };
      }
      
      // Actualizar badge de conectado
      const ssidDiv = card.querySelector('.network-card-ssid');
      if (ssidDiv) {
        if (isConnected) {
          if (!ssidDiv.querySelector('.badge')) {
            const badge = document.createElement('span');
            badge.className = 'badge bg-success ms-2';
            badge.textContent = t('wifi.connected', 'Connected');
            ssidDiv.appendChild(badge);
          }
          } else {
          const badge = ssidDiv.querySelector('.badge');
          if (badge) badge.remove();
        }
      }
    });
  }

  // Desconectar WiFi
  async function disconnectWiFi(buttonElement) {
    if (!buttonElement) return;
    
    const originalText = buttonElement.innerHTML;
    buttonElement.disabled = true;
    buttonElement.innerHTML = `<i class="bi bi-arrow-clockwise spinning me-2"></i>${t('wifi.disconnecting', 'Disconnecting...')}`;
    
    try {
      // Intentar endpoint v1 y fallback legacy para compatibilidad
      let resp = await apiRequest('/api/v1/wifi/disconnect', { method: 'POST' });
      if (resp && resp.status === 404) {
        resp = await apiRequest('/api/wifi/disconnect', { method: 'POST' });
      }
      const data = await resp.json();
      
      if (resp.ok && data.success) {
        showAlert('success', t('wifi.disconnected', 'Disconnected from WiFi'));
        currentConnectedSSID = null;
        await loadConnectionStatus();
        // Actualizar botones después de un breve delay
    setTimeout(() => {
          updateConnectButtons();
      scanNetworks();
        }, 1000);
      } else {
        showAlert('danger', translateError(data.error) || t('wifi.disconnect_error', 'Error disconnecting from WiFi'));
        buttonElement.disabled = false;
        buttonElement.innerHTML = originalText;
      }
    } catch (error) {
      console.error(t('wifi.disconnect_error', 'Error disconnecting from WiFi') + ':', error);
      showAlert('danger', t('wifi.disconnect_error', 'Error disconnecting from WiFi'));
      buttonElement.disabled = false;
      buttonElement.innerHTML = originalText;
    }
  }

  // Exportar funciones globales
  window.loadConnectionStatus = loadConnectionStatus;
  window.toggleWiFi = toggleWiFi;
  window.unblockWiFi = unblockWiFi;
  window.toggleSoftwareSwitch = toggleSoftwareSwitch;
  window.scanNetworks = scanNetworks;

  // Actualizar textos de botones según el estado
  function updateButtonTexts(data) {
    // Botón toggle WiFi
    const toggleWifiText = document.getElementById('toggle-wifi-text');
    const toggleWifiIcon = document.getElementById('toggle-wifi-icon');
    if (toggleWifiText && data.enabled !== undefined) {
      toggleWifiText.textContent = data.enabled 
        ? t('wifi.disable_wifi', 'Disable WiFi')
        : t('wifi.enable_wifi', 'Enable WiFi');
    }
    if (toggleWifiIcon && data.enabled !== undefined) {
      toggleWifiIcon.className = data.enabled ? 'bi bi-wifi-off' : 'bi bi-wifi';
    }
    
    // Botón software switch
    const softwareSwitchText = document.getElementById('software-switch-text');
    const softwareSwitchIcon = document.getElementById('software-switch-icon');
    if (softwareSwitchText && data.soft_blocked !== undefined) {
      softwareSwitchText.textContent = data.soft_blocked
        ? t('wifi.enable_software_switch', 'Enable Software Switch')
        : t('wifi.disable_software_switch', 'Disable Software Switch');
    }
    if (softwareSwitchIcon && data.soft_blocked !== undefined) {
      softwareSwitchIcon.className = data.soft_blocked ? 'bi bi-toggle-off' : 'bi bi-toggle-on';
    }
  }

  // Inicializar cuando el DOM esté listo
  document.addEventListener('DOMContentLoaded', async function() {
    await loadInterfaces();
            await loadConnectionStatus();
    
    // Verificar si WiFi está bloqueado y actualizar botones
    try {
      const resp = await apiRequest('/api/v1/wifi/status');
      if (resp && resp.ok) {
        const data = await resp.json();
        updateButtonTexts(data);
        const unblockBtn = document.getElementById('unblock-wifi-btn');
        if (unblockBtn && data.hard_blocked) {
          unblockBtn.style.display = 'block';
        }
      }
    } catch (error) {
      console.error(t('wifi.status_error', 'Error getting WiFi status') + ':', error);
    }
  });
})();
