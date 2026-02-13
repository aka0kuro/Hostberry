// HostAPD Page JavaScript
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
      'hostapd toggle no implementado': t('hostapd.toggle_not_implemented', 'HostAPD toggle not implemented'),
      'hostapd restart no implementado': t('hostapd.restart_not_implemented', 'HostAPD restart not implemented'),
      'hostapd config no implementado': t('hostapd.config_not_implemented', 'HostAPD config not implemented'),
      'error loading access points': t('hostapd.error_loading_aps', 'Error loading access points'),
      'error loading clients': t('hostapd.error_loading_clients', 'Error loading clients'),
      'operation failed': t('errors.operation_failed', 'Operation failed'),
      'network error': t('errors.network_error', 'Network error'),
    };
    
    for (const [key, value] of Object.entries(errorMap)) {
      if (errorLower.includes(key)) {
        return value;
      }
    }
    
    return errorMessage;
  };

  // Cargar estado de HostAPD
  async function loadHostAPDStatus() {
    const container = document.getElementById('hostapdStatusContainer');
    if (!container) return;
    
    try {
      // Intentar obtener el estado del servicio
      const resp = await HostBerry.apiRequest('/api/v1/system/services');
      if (resp && resp.ok) {
        const data = await resp.json();
        const hostapd = data.services?.hostapd || {};
        const enabled = hostapd.enabled === true || hostapd.enabled === 'enabled'; // Si está habilitado para iniciar al arranque
        const running = hostapd.status === 'active' || hostapd.active === true; // Si está corriendo actualmente
        
        // Actualizar UI
        updateStatusUI(enabled, running);
      } else {
        // Si no hay servicio, mostrar estado desconocido
        updateStatusUI(false, false);
      }
    } catch (e) {
      console.error('Error loading HostAPD status:', e);
      updateStatusUI(false, false);
    }
  }

  function updateStatusUI(enabled, running) {
    // Actualizar tarjeta de estado
    const statusValue = document.getElementById('hostapd-status-value');
    const statusBar = document.getElementById('hostapd-status-bar');
    const statusIcon = document.getElementById('hostapd-status-icon');
    const toggleBtn = document.getElementById('toggle-hostapd-btn');
    const toggleText = document.getElementById('toggle-hostapd-text');
    const container = document.getElementById('hostapdStatusContainer');
    
    // Usar 'running' como el estado real del servicio (más confiable que 'enabled')
    const isActuallyActive = running || enabled;
    
    // Nota: loadInterfaces se llama en init y tras crear ap0; evadir aquí para no saturar API
    if (statusValue) {
      statusValue.textContent = isActuallyActive ? t('hostapd.enabled', 'Enabled') : t('hostapd.disabled', 'Disabled');
      statusValue.className = 'stat-value' + (isActuallyActive ? ' text-success' : ' text-danger');
    }
    
    if (statusBar) {
      statusBar.style.width = isActuallyActive ? '100%' : '0%';
      statusBar.className = 'stat-progress-bar' + (isActuallyActive ? ' bg-success' : ' bg-danger');
    }
    
    if (statusIcon) {
      statusIcon.className = 'bi ' + (isActuallyActive ? 'bi-heart-pulse-fill text-success' : 'bi-heart-pulse text-danger');
    }
    
    if (toggleBtn) {
      // Si está corriendo, mostrar botón para detener
      if (running) {
        toggleBtn.className = 'btn btn-outline-danger';
        toggleBtn.disabled = false;
      } 
      // Si está habilitado pero no corriendo, mostrar botón para iniciar
      else if (enabled && !running) {
        toggleBtn.className = 'btn btn-success';
        toggleBtn.disabled = false;
      }
      // Si no está habilitado ni corriendo, mostrar botón para habilitar e iniciar
      else {
        toggleBtn.className = 'btn btn-outline-success';
        toggleBtn.disabled = false;
      }
    }
    
    if (toggleText) {
      if (running) {
        toggleText.innerHTML = t('hostapd.stop_hostapd', 'Stop HostAPD');
      } else if (enabled && !running) {
        toggleText.innerHTML = t('hostapd.start_service', 'Start Service');
      } else {
        toggleText.innerHTML = t('hostapd.enable_hostapd', 'Enable HostAPD');
      }
    } else if (toggleBtn) {
      // Si no hay toggleText, actualizar el contenido del botón directamente
      let icon, text;
      if (running) {
        icon = 'bi-stop-circle';
        text = t('hostapd.stop_hostapd', 'Stop HostAPD');
      } else if (enabled && !running) {
        icon = 'bi-play-circle';
        text = t('hostapd.start_service', 'Start Service');
      } else {
        icon = 'bi-router';
        text = t('hostapd.enable_hostapd', 'Enable HostAPD');
      }
      toggleBtn.innerHTML = '<i class="bi ' + icon + ' me-2"></i><span>' + text + '</span>';
    }
    
    if (container) {
      container.innerHTML = `
        <div class="info-grid">
          <div class="info-item">
            <span class="info-label">${t('hostapd.hostapd_status', 'HostAPD Status')}</span>
            <span class="info-value ${enabled ? 'text-success' : 'text-danger'}">
              <i class="bi ${enabled ? 'bi-check-circle' : 'bi-x-circle'} me-1"></i>
              ${enabled ? t('hostapd.enabled', 'Enabled') : t('hostapd.disabled', 'Disabled')}
            </span>
          </div>
          <div class="info-item">
            <span class="info-label">${t('hostapd.service_status', 'Service Status')}</span>
            <span class="info-value ${running ? 'text-success' : 'text-danger'}">
              <i class="bi ${running ? 'bi-check-circle' : 'bi-x-circle'} me-1"></i>
              ${running ? t('hostapd.running', 'Running') : t('hostapd.stopped', 'Stopped')}
            </span>
          </div>
          ${!running ? `
          <div class="info-item">
            <span class="info-label text-muted small">${t('hostapd.tip', 'Tip')}</span>
            <span class="info-value text-muted small">
              ${enabled ? t('hostapd.click_start', 'Click "Start Service" button above to start the service') : t('hostapd.click_enable', 'Click "Enable HostAPD" to start the service')}
            </span>
          </div>
          ` : ''}
        </div>
      `;
    }
  }

  // Cargar puntos de acceso
  async function loadAccessPoints() {
    const container = document.getElementById('accessPointsContainer');
    if (!container) return;
    
    container.innerHTML = '<div class="text-center py-4"><div class="spinning mb-3"><i class="bi bi-arrow-clockwise" style="font-size: 2rem;"></i></div><p class="text-muted">' + t('common.loading', 'Loading...') + '</p></div>';
    
    try {
      const resp = await HostBerry.apiRequest('/api/v1/hostapd/access-points');
      if (resp && resp.ok) {
        const aps = await resp.json();
        const countEl = document.getElementById('access-points-count');
        if (countEl) countEl.textContent = Array.isArray(aps) ? aps.length : 0;
        
        if (!Array.isArray(aps) || aps.length === 0) {
          container.innerHTML = `
            <div class="text-center py-4">
              <i class="bi bi-wifi text-muted" style="font-size: 3rem;"></i>
              <p class="text-muted mt-3">${t('hostapd.no_access_points', 'No access points configured')}</p>
            </div>
          `;
          return;
        }
        
        let html = '<div class="table-responsive"><table class="table table-hover"><thead><tr>';
        html += `<th>${t('hostapd.access_point_name', 'Name')}</th>`;
        html += `<th>${t('hostapd.access_point_ssid', 'SSID')}</th>`;
        html += `<th>${t('hostapd.access_point_status', 'Status')}</th>`;
        html += `<th>${t('hostapd.access_point_clients', 'Clients')}</th>`;
        html += `<th>${t('common.actions', 'Actions')}</th>`;
        html += '</tr></thead><tbody>';
        
        aps.forEach(ap => {
          // Verificar estado: usar 'active' si está disponible, sino 'enabled', sino verificar 'status'
          const isActive = ap.active === true || 
                          (ap.enabled === true || ap.enabled === 'true') ||
                          (ap.status === 'active');
          
          // Verificar si el servicio está corriendo pero no transmite
          const serviceRunning = ap.service_running === true;
          const transmitting = ap.transmitting === true;
          const status = ap.status || 'inactive';
          
          // Determinar el estado visual
          let statusBadge = '';
          let statusText = '';
          if (isActive && transmitting) {
            statusBadge = 'success';
            statusText = t('hostapd.active', 'Active');
          } else if (serviceRunning && !transmitting) {
            statusBadge = 'warning';
            statusText = t('hostapd.service_running_not_transmitting', 'Running but not transmitting');
          } else if (status === 'error') {
            statusBadge = 'danger';
            statusText = t('hostapd.error', 'Error');
          } else {
            statusBadge = 'danger';
            statusText = t('hostapd.inactive', 'Inactive');
          }
          const apName = String(ap.name || ap.interface || '-');
          const apSsid = String(ap.ssid || '-');
          const configTarget = String(ap.name || ap.ssid || '');
          const configTargetEncoded = encodeURIComponent(configTarget);
          const clientsCount = Number.isFinite(Number(ap.clients_count)) ? Number(ap.clients_count) : 0;
          
          html += '<tr>';
          html += `<td>${escapeHtml(apName)}</td>`;
          html += `<td>${escapeHtml(apSsid)}</td>`;
          html += `<td><span class="badge bg-${statusBadge}" title="${serviceRunning && !transmitting ? t('hostapd.check_diagnostics', 'Service is running but WiFi network is not visible. Check diagnostics.') : ''}">${statusText}</span></td>`;
          html += `<td>${clientsCount}</td>`;
          html += `<td><button class="btn btn-sm btn-outline-primary" type="button" data-config-ap="${configTargetEncoded}"><i class="bi bi-gear"></i></button></td>`;
          html += '</tr>';
        });
        
        html += '</tbody></table></div>';
        container.innerHTML = html;
        container.querySelectorAll('button[data-config-ap]').forEach((btn) => {
          btn.addEventListener('click', () => {
            const rawTarget = btn.getAttribute('data-config-ap') || '';
            try {
              configureAccessPoint(decodeURIComponent(rawTarget));
            } catch (_e) {
              configureAccessPoint(rawTarget);
            }
          });
        });
        
        // Mostrar diagnósticos si algún AP está corriendo pero no transmite
        const hasNotTransmitting = aps.some(ap => ap.service_running === true && ap.transmitting !== true);
        if (hasNotTransmitting) {
          const diagnosticsSection = document.getElementById('diagnosticsSection');
          if (diagnosticsSection) {
            diagnosticsSection.style.display = 'block';
            loadDiagnostics();
          }
        } else {
          const diagnosticsSection = document.getElementById('diagnosticsSection');
          if (diagnosticsSection) {
            diagnosticsSection.style.display = 'none';
          }
        }
      } else {
        const errorText = await resp.text().catch(() => '');
        container.innerHTML = `
          <div class="text-center py-4">
            <i class="bi bi-exclamation-triangle text-warning" style="font-size: 3rem;"></i>
            <p class="text-muted mt-3">${translateError(errorText) || t('hostapd.error_loading_aps', 'Error loading access points')}</p>
          </div>
        `;
        if (resp.status >= 500) {
          HostBerry.showAlert('danger', translateError(errorText) || t('hostapd.error_loading_aps', 'Error loading access points'));
        }
      }
    } catch (e) {
      console.error('Error loading access points:', e);
      container.innerHTML = `
        <div class="text-center py-4">
          <i class="bi bi-exclamation-triangle text-danger" style="font-size: 3rem;"></i>
          <p class="text-muted mt-3">${t('errors.network_error', 'Network error')}</p>
        </div>
      `;
      HostBerry.showAlert('danger', t('errors.network_error', 'Network error'));
    }
  }

  // Cargar clientes
  async function loadClients() {
    const container = document.getElementById('clientsContainer');
    if (!container) return;
    
    container.innerHTML = '<div class="text-center py-4"><div class="spinning mb-3"><i class="bi bi-arrow-clockwise" style="font-size: 2rem;"></i></div><p class="text-muted">' + t('common.loading', 'Loading...') + '</p></div>';
    
    try {
      const resp = await HostBerry.apiRequest('/api/v1/hostapd/clients');
      if (resp && resp.ok) {
        const clients = await resp.json();
        const countEl = document.getElementById('clients-count');
        if (countEl) countEl.textContent = Array.isArray(clients) ? clients.length : 0;
        
        if (!Array.isArray(clients) || clients.length === 0) {
          container.innerHTML = `
            <div class="text-center py-4">
              <i class="bi bi-people text-muted" style="font-size: 3rem;"></i>
              <p class="text-muted mt-3">${t('hostapd.no_clients', 'No clients connected')}</p>
            </div>
          `;
          return;
        }
        
        let html = '<div class="table-responsive"><table class="table table-hover"><thead><tr>';
        html += `<th>${t('hostapd.client_mac', 'MAC Address')}</th>`;
        html += `<th>${t('hostapd.client_ip', 'IP Address')}</th>`;
        html += `<th>${t('hostapd.client_signal', 'Signal')}</th>`;
        html += `<th>${t('hostapd.client_uptime', 'Uptime')}</th>`;
        html += '</tr></thead><tbody>';
        
        clients.forEach(client => {
          const safeMac = escapeHtml(client.mac_address || client.mac || '-');
          const safeIp = escapeHtml(client.ip_address || client.ip || '-');
          const safeSignal = escapeHtml(client.signal ? client.signal + ' dBm' : '-');
          const safeUptime = escapeHtml(client.uptime || '-');
          html += '<tr>';
          html += `<td>${safeMac}</td>`;
          html += `<td>${safeIp}</td>`;
          html += `<td>${safeSignal}</td>`;
          html += `<td>${safeUptime}</td>`;
          html += '</tr>';
        });
        
        html += '</tbody></table></div>';
        container.innerHTML = html;
      } else {
        const errorText = await resp.text().catch(() => '');
        container.innerHTML = `
          <div class="text-center py-4">
            <i class="bi bi-exclamation-triangle text-warning" style="font-size: 3rem;"></i>
            <p class="text-muted mt-3">${translateError(errorText) || t('hostapd.error_loading_clients', 'Error loading clients')}</p>
          </div>
        `;
        if (resp.status >= 500) {
          HostBerry.showAlert('danger', translateError(errorText) || t('hostapd.error_loading_clients', 'Error loading clients'));
        }
      }
    } catch (e) {
      console.error('Error loading clients:', e);
      container.innerHTML = `
        <div class="text-center py-4">
          <i class="bi bi-exclamation-triangle text-danger" style="font-size: 3rem;"></i>
          <p class="text-muted mt-3">${t('errors.network_error', 'Network error')}</p>
        </div>
      `;
      HostBerry.showAlert('danger', t('errors.network_error', 'Network error'));
    }
  }

  // Toggle HostAPD
  window.toggleHostAPD = async function() {
    const btn = document.getElementById('toggle-hostapd-btn');
    const toggleText = document.getElementById('toggle-hostapd-text');
    let originalBtnClass = '';
    let originalText = '';
    
    if (btn) {
      btn.disabled = true;
      originalBtnClass = btn.className;
      originalText = toggleText ? toggleText.textContent : btn.textContent;
      if (toggleText) {
        toggleText.innerHTML = '<i class="bi bi-arrow-clockwise spinning me-2"></i>' + t('common.loading', 'Loading...');
      } else {
        btn.innerHTML = '<i class="bi bi-arrow-clockwise spinning me-2"></i>' + t('common.loading', 'Loading...');
      }
    }
    
    const restoreButton = () => {
      if (btn) {
        btn.disabled = false;
        // Recargar el estado para actualizar el botón correctamente
        // Usar un delay más largo para asegurar que el estado del servidor se haya actualizado
        setTimeout(() => {
          loadHostAPDStatus();
        }, 1500);
      }
    };
    
    try {
      const resp = await HostBerry.apiRequest('/api/v1/hostapd/toggle', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json'
        }
      });
      
      if (resp && resp.ok) {
        let result;
        try {
          result = await resp.json();
        } catch (jsonErr) {
          console.error('Error parsing JSON response:', jsonErr);
          const text = await resp.text().catch(() => '');
          console.error('HostAPD toggle non-JSON response:', text);
          HostBerry.showAlert('warning', t('errors.unexpected_response', 'Unexpected response from server'));
          restoreButton();
          return;
        }
        
        if (result.error) {
          // Si falta la configuración, mostrar mensaje más claro
          if (result.config_missing) {
            const configMsg = t('hostapd.config_required', 'HostAPD configuration required. Please configure HostAPD first using the configuration form below.');
            HostBerry.showAlert('warning', configMsg);
            // Scroll suave hacia el formulario de configuración
            setTimeout(() => {
              const configForm = document.getElementById('hostapdConfigForm');
              if (configForm) {
                configForm.scrollIntoView({ behavior: 'smooth', block: 'center' });
                // Resaltar el formulario brevemente
                configForm.style.transition = 'box-shadow 0.3s ease';
                const wrgb = getComputedStyle(document.documentElement).getPropertyValue('--hb-warning-rgb').trim() || '239, 68, 68';
                configForm.style.boxShadow = '0 0 20px rgba(' + wrgb + ', 0.5)';
                setTimeout(() => {
                  configForm.style.boxShadow = '';
                }, 2000);
              }
            }, 500);
          } else {
            HostBerry.showAlert('warning', translateError(result.error));
          }
          restoreButton();
        } else {
          // Usar el estado real del servicio desde la respuesta
          const actuallyEnabled = result.enabled === true;
          const action = actuallyEnabled ? t('hostapd.enabled', 'Enabled') : t('hostapd.disabled', 'Disabled');
          const statusMsg = result.status ? ` (${result.status})` : '';
          
          if (actuallyEnabled) {
            HostBerry.showAlert('success', t('hostapd.hostapd_status_changed', 'HostAPD {status}').replace('{status}', action) + statusMsg);
          } else {
            // Si se intentó activar pero no se activó, mostrar advertencia
            if (result.action === 'enable') {
              const errorMsg = result.error || t('hostapd.enable_failed', 'Failed to enable HostAPD. Check configuration and logs.');
              HostBerry.showAlert('warning', errorMsg + statusMsg);
            } else {
              HostBerry.showAlert('success', t('hostapd.hostapd_status_changed', 'HostAPD {status}').replace('{status}', action) + statusMsg);
            }
          }
          
          // Restaurar el botón y recargar el estado
          restoreButton();
          
          // Recargar el estado después de un delay para dar tiempo al servicio
          setTimeout(() => {
            loadHostAPDStatus(); // Recargar estado del servicio
            loadAccessPoints();
            loadClients();
          }, 2000); // Aumentar delay para dar más tiempo al servicio
        }
      } else {
        const status = resp ? resp.status : 'unknown';
        console.error('HostAPD toggle failed with status:', status);
        let errorText = '';
        try {
          errorText = await resp.text();
        } catch (e) {
          errorText = `HTTP ${status}`;
        }
        console.error('Error response:', errorText);
        HostBerry.showAlert('danger', translateError(errorText) || t('errors.operation_failed', 'Operation failed'));
        restoreButton();
      }
    } catch (e) {
      console.error('Error toggling HostAPD:', e);
      HostBerry.showAlert('danger', t('errors.network_error', 'Network error') + ': ' + (e.message || String(e)));
      restoreButton();
    }
  };

  // Cargar diagnósticos de HostAPD
  window.loadDiagnostics = async function() {
    const container = document.getElementById('diagnosticsContainer');
    if (!container) return;
    
    container.innerHTML = '<div class="text-center py-4"><div class="spinning mb-3"><i class="bi bi-arrow-clockwise" style="font-size: 2rem;"></i></div><p class="text-muted">' + t('common.loading', 'Loading...') + '</p></div>';
    
    try {
      const resp = await HostBerry.apiRequest('/api/v1/hostapd/diagnostics');
      if (resp && resp.ok) {
        const diagnostics = await resp.json();
        
        let html = '<div class="diagnostics-content">';
        
        // Estado del servicio
        html += '<div class="mb-4">';
        html += '<h6 class="fw-bold mb-3"><i class="bi bi-info-circle me-2"></i>' + t('hostapd.service_status', 'Service Status') + '</h6>';
        html += '<div class="row g-2">';
        html += '<div class="col-md-6"><strong>' + t('hostapd.service_running', 'Service Running') + ':</strong> <span class="badge bg-' + (diagnostics.service_running ? 'success' : 'danger') + '">' + (diagnostics.service_running ? t('common.yes', 'Yes') : t('common.no', 'No')) + '</span></div>';
        html += '<div class="col-md-6"><strong>' + t('hostapd.transmitting', 'Transmitting') + ':</strong> <span class="badge bg-' + (diagnostics.transmitting ? 'success' : 'warning') + '">' + (diagnostics.transmitting ? t('common.yes', 'Yes') : t('common.no', 'No')) + '</span></div>';
        html += '<div class="col-md-6"><strong>' + t('hostapd.interface', 'Interface') + ':</strong> <code>' + escapeHtml(diagnostics.interface || 'N/A') + '</code></div>';
        html += '<div class="col-md-6"><strong>' + t('hostapd.interface_up', 'Interface Up') + ':</strong> <span class="badge bg-' + (diagnostics.interface_up ? 'success' : 'danger') + '">' + (diagnostics.interface_up ? t('common.yes', 'Yes') : t('common.no', 'No')) + '</span></div>';
        html += '<div class="col-md-6"><strong>' + t('hostapd.interface_in_ap_mode', 'Interface in AP Mode') + ':</strong> <span class="badge bg-' + (diagnostics.interface_in_ap_mode ? 'success' : 'warning') + '">' + (diagnostics.interface_in_ap_mode ? t('common.yes', 'Yes') : t('common.no', 'No')) + '</span></div>';
        html += '<div class="col-md-6"><strong>DNSmasq:</strong> <span class="badge bg-' + (diagnostics.dnsmasq_running ? 'success' : 'warning') + '">' + (diagnostics.dnsmasq_running ? t('common.running', 'Running') : t('common.stopped', 'Stopped')) + '</span></div>';
        html += '</div>';
        html += '</div>';
        
        // Errores detectados
        if (diagnostics.has_errors && diagnostics.errors && diagnostics.errors.length > 0) {
          html += '<div class="mb-4">';
          html += '<h6 class="fw-bold mb-3 text-danger"><i class="bi bi-exclamation-triangle me-2"></i>' + t('hostapd.errors_detected', 'Errors Detected') + '</h6>';
          html += '<ul class="list-unstyled">';
          diagnostics.errors.forEach(error => {
            html += '<li class="mb-2"><span class="badge bg-danger me-2">!</span>' + escapeHtml(String(error || '')) + '</li>';
          });
          html += '</ul>';
          html += '</div>';
        }
        
        // Logs recientes
        if (diagnostics.recent_logs) {
          html += '<div class="mb-4">';
          html += '<h6 class="fw-bold mb-3"><i class="bi bi-file-text me-2"></i>' + t('hostapd.recent_logs', 'Recent Logs') + '</h6>';
          html += '<pre class="bg-dark text-light p-3 rounded" style="max-height: 300px; overflow-y: auto; font-size: 0.85rem;">' + escapeHtml(diagnostics.recent_logs.substring(0, 2000)) + '</pre>';
          html += '</div>';
        }
        
        // Sugerencias de solución
        html += '<div class="alert alert-info">';
        html += '<h6 class="fw-bold mb-2"><i class="bi bi-lightbulb me-2"></i>' + t('hostapd.troubleshooting_tips', 'Troubleshooting Tips') + '</h6>';
        html += '<ul class="mb-0">';
        
        if (!diagnostics.interface_up) {
          html += '<li>' + t('hostapd.tip_interface_down', 'The network interface is down. Try: sudo ip link set {interface} up').replace('{interface}', escapeHtml(diagnostics.interface || 'wlan0')) + '</li>';
        }
        if (!diagnostics.interface_in_ap_mode && diagnostics.service_running) {
          html += '<li>' + t('hostapd.tip_driver_issue', 'The interface is not in AP mode. This may indicate a driver issue. Check if your WiFi adapter supports AP mode.') + '</li>';
        }
        if (diagnostics.has_errors) {
          html += '<li>' + t('hostapd.tip_check_logs', 'Check the logs above for specific error messages. Common issues: driver not supporting AP mode, interface already in use, or incorrect channel.') + '</li>';
        }
        if (!diagnostics.dnsmasq_running && diagnostics.service_running) {
          html += '<li>' + t('hostapd.tip_dnsmasq', 'DNSmasq is not running. This is needed for DHCP. Try: sudo systemctl start dnsmasq') + '</li>';
        }
        if (diagnostics.service_running && !diagnostics.transmitting && !diagnostics.has_errors) {
          html += '<li>' + t('hostapd.tip_restart', 'Try restarting HostAPD: sudo systemctl restart hostapd') + '</li>';
          html += '<li>' + t('hostapd.tip_check_wifi', 'Make sure the WiFi interface is not being used by NetworkManager or another service. Try: sudo systemctl stop NetworkManager') + '</li>';
        }
        
        html += '</ul>';
        html += '</div>';
        
        html += '</div>';
        container.innerHTML = html;
      } else {
        const errorText = await resp.text().catch(() => '');
        container.innerHTML = `
          <div class="text-center py-4">
            <i class="bi bi-exclamation-triangle text-warning" style="font-size: 3rem;"></i>
            <p class="text-muted mt-3">${translateError(errorText) || t('hostapd.error_loading_diagnostics', 'Error loading diagnostics')}</p>
          </div>
        `;
      }
    } catch (e) {
      console.error('Error loading diagnostics:', e);
      container.innerHTML = `
        <div class="text-center py-4">
          <i class="bi bi-exclamation-triangle text-danger" style="font-size: 3rem;"></i>
          <p class="text-muted mt-3">${t('errors.network_error', 'Network error')}: ${e.message || String(e)}</p>
        </div>
      `;
    }
  };
  
  // Función auxiliar para escapar HTML (usa HostBerry.escapeHtml si está disponible)
  function escapeHtml(text) {
    if (text == null) return '';
    const s = String(text);
    if (window.HostBerry && window.HostBerry.escapeHtml) return window.HostBerry.escapeHtml(s);
    const div = document.createElement('div');
    div.textContent = s;
    return div.innerHTML;
  }

  // Restart HostAPD
  window.restartHostAPD = async function() {
    if (!confirm(t('hostapd.confirm_restart', 'Are you sure you want to restart HostAPD?'))) {
      return;
    }
    
    try {
      const resp = await HostBerry.apiRequest('/api/v1/hostapd/restart', {
        method: 'POST'
      });
      
      if (resp && resp.ok) {
        const result = await resp.json();
        if (result.error) {
          HostBerry.showAlert('warning', translateError(result.error));
        } else {
          HostBerry.showAlert('success', t('messages.operation_successful', 'Operation successful'));
          setTimeout(() => {
            loadHostAPDStatus();
            loadAccessPoints();
            loadClients();
          }, 2000);
        }
      } else {
        const errorText = await resp.text().catch(() => '');
        HostBerry.showAlert('danger', translateError(errorText) || t('errors.operation_failed', 'Operation failed'));
      }
    } catch (e) {
      console.error('Error restarting HostAPD:', e);
      HostBerry.showAlert('danger', t('errors.network_error', 'Network error'));
    }
  };

  // Configure Access Point
  window.configureAccessPoint = function(apName) {
    const form = document.getElementById('hostapdConfigForm');
    const ssidInput = document.getElementById('hostapd-ssid');
    const targetName = String(apName || '').trim();

    if (targetName && ssidInput) {
      ssidInput.value = targetName;
    }

    if (form) {
      form.scrollIntoView({ behavior: 'smooth', block: 'center' });
      form.classList.add('border', 'border-info');
      setTimeout(() => form.classList.remove('border', 'border-info'), 1200);
    }

    HostBerry.showAlert('info', t('hostapd.configuring_access_point', 'Configuring access point') + (targetName ? ': ' + targetName : ''));
  };

  // Compatibilidad con botones inline de la plantilla
  window.loadHostAPDStatus = loadHostAPDStatus;
  window.loadAccessPoints = loadAccessPoints;
  window.loadClients = loadClients;

  // Toggle password visibility
  window.toggleHostAPDPassword = function() {
    const passwordInput = document.getElementById('hostapd-password');
    const toggleIcon = document.getElementById('hostapd-password-toggle-icon');
    if (passwordInput && toggleIcon) {
      if (passwordInput.type === 'password') {
        passwordInput.type = 'text';
        toggleIcon.className = 'bi bi-eye-slash';
      } else {
        passwordInput.type = 'password';
        toggleIcon.className = 'bi bi-eye';
      }
    }
  };

  // Cargar interfaces WiFi para el selector (solo ap0)
  async function loadInterfaces() {
    const select = document.getElementById('hostapd-interface');
    if (!select) return Promise.resolve();
    
    try {
      // Obtener todas las interfaces de red para verificar si ap0 existe
      const resp = await HostBerry.apiRequest('/api/v1/network/interfaces');
      let ap0Exists = false;
      
      if (resp && resp.ok) {
        const data = await resp.json();
        let interfaces = [];
        if (Array.isArray(data)) {
          interfaces = data;
        } else if (data.interfaces && Array.isArray(data.interfaces)) {
          interfaces = data.interfaces;
        } else if (data.data && Array.isArray(data.data)) {
          interfaces = data.data;
        }
        
        // Buscar ap0 en la lista de interfaces
        ap0Exists = interfaces.some(iface => {
          const name = (typeof iface === 'object' && iface !== null) 
            ? (iface.name || iface.interface || iface.device || '')
            : String(iface);
          return name === 'ap0';
        });
      }
      
      // Siempre mostrar ap0 en el selector (es la única interfaz válida para HostAPD)
      // Si no existe, se creará automáticamente cuando se configure HostAPD
      select.innerHTML = '<option value="">' + t('hostapd.select_interface', 'Select Interface') + '</option>';
      const option = document.createElement('option');
      option.value = 'ap0';
      option.textContent = 'ap0';
      if (!ap0Exists) {
        // Si no existe, agregar un indicador visual
        option.textContent = 'ap0 (' + t('hostapd.will_be_created', 'will be created') + ')';
      }
      select.appendChild(option);
      
      return Promise.resolve();
    } catch (e) {
      console.error('Error loading interfaces:', e);
      // En caso de error, mostrar ap0 de todas formas
      const select = document.getElementById('hostapd-interface');
      if (select) {
        select.innerHTML = '<option value="">' + t('hostapd.select_interface', 'Select Interface') + '</option>';
        const option = document.createElement('option');
        option.value = 'ap0';
        option.textContent = 'ap0';
        select.appendChild(option);
      }
      return Promise.resolve();
    }
  }

  // Cargar configuración de HostAPD y rellenar el formulario
  async function loadHostAPDConfig() {
    try {
      const resp = await HostBerry.apiRequest('/api/v1/hostapd/config');
      if (resp && resp.ok) {
        const result = await resp.json();
        if (result.success && result.config) {
          const config = result.config;
          
          // Rellenar campos del formulario
          if (config.interface) {
            const interfaceSelect = document.getElementById('hostapd-interface');
            if (interfaceSelect) {
              // Esperar a que las interfaces se carguen
              setTimeout(() => {
                interfaceSelect.value = config.interface;
              }, 500);
            }
          }
          
          if (config.ssid) {
            const ssidInput = document.getElementById('hostapd-ssid');
            if (ssidInput) ssidInput.value = config.ssid;
          }
          
          if (config.channel) {
            const channelSelect = document.getElementById('hostapd-channel');
            if (channelSelect) channelSelect.value = config.channel;
          }
          
          if (config.security) {
            const securitySelect = document.getElementById('hostapd-security');
            if (securitySelect) securitySelect.value = config.security;
          }
          
          if (config.gateway) {
            const gatewayInput = document.getElementById('hostapd-gateway');
            if (gatewayInput) gatewayInput.value = config.gateway;
          }
          
          if (config.dhcp_range_start) {
            const dhcpStartInput = document.getElementById('hostapd-dhcp-start');
            if (dhcpStartInput) dhcpStartInput.value = config.dhcp_range_start;
          }
          
          if (config.dhcp_range_end) {
            const dhcpEndInput = document.getElementById('hostapd-dhcp-end');
            if (dhcpEndInput) dhcpEndInput.value = config.dhcp_range_end;
          }
          
          if (config.lease_time) {
            const leaseTimeInput = document.getElementById('hostapd-lease-time');
            if (leaseTimeInput) leaseTimeInput.value = config.lease_time;
          }
          
          if (config.country) {
            const countrySelect = document.getElementById('hostapd-country');
            if (countrySelect) countrySelect.value = config.country;
          }
          
          // Nota: No cargamos la contraseña por seguridad
        }
      }
    } catch (e) {
      console.error('Error loading HostAPD config:', e);
      // No mostrar error si el archivo no existe (primera vez)
    }
  }

  // Inicializar página
  function initHostAPDPage() {
    // Esperar a que HostBerry esté disponible
    function waitForHostBerry(callback) {
      if (window.HostBerry && window.HostBerry.apiRequest) {
        callback();
      } else {
        setTimeout(() => waitForHostBerry(callback), 100);
      }
    }
    
    waitForHostBerry(async () => {
      // Crear ap0 si no existe
      try {
        const ap0Resp = await HostBerry.apiRequest('/api/v1/hostapd/create-ap0', {
          method: 'POST'
        });
        if (ap0Resp && ap0Resp.ok) {
          await ap0Resp.json();
        }
      } catch (e) {
        console.error('Error creating ap0 interface:', e);
      }
      
      loadHostAPDStatus();
      loadAccessPoints();
      loadClients();
      
      // Cargar interfaces primero, luego la configuración
      await loadInterfaces();
      // Recargar interfaces periódicamente para detectar cuando ap0 se active
      setInterval(async () => {
        await loadInterfaces();
      }, 5000); // Cada 5 segundos
      // Esperar un poco más para que el selector se actualice
      setTimeout(() => {
        loadHostAPDConfig(); // Cargar configuración existente
      }, 300);
      
      // Configurar formulario
      const form = document.getElementById('hostapdConfigForm');
      if (form) {
        form.addEventListener('submit', async function(e) {
          e.preventDefault();
          
          const submitBtn = form.querySelector('button[type="submit"]');
          const originalBtnText = submitBtn ? submitBtn.innerHTML : '';
          
          // Deshabilitar botón y mostrar loading
          if (submitBtn) {
            submitBtn.disabled = true;
            submitBtn.innerHTML = '<i class="bi bi-arrow-clockwise spinning me-2"></i>' + (t('common.saving', 'Saving...') || 'Saving...');
          }
          
          const fd = new FormData(this);
          const data = {
            interface: fd.get('interface'),
            ssid: fd.get('ssid'),
            password: fd.get('password') || '',
            channel: parseInt(fd.get('channel')),
            security: fd.get('security'),
            gateway: fd.get('gateway') || '192.168.4.1',
            dhcp_range_start: fd.get('dhcp_range_start') || '192.168.4.2',
            dhcp_range_end: fd.get('dhcp_range_end') || '192.168.4.254',
            lease_time: fd.get('lease_time') || '12h',
            country: fd.get('country') || 'US'
          };
          
          // Validar campos requeridos
          if (!data.interface || !data.ssid || !data.channel) {
            HostBerry.showAlert('warning', t('hostapd.fill_required_fields', 'Please fill in all required fields: Interface, SSID, and Channel'));
            if (submitBtn) {
              submitBtn.disabled = false;
              submitBtn.innerHTML = originalBtnText;
            }
            return;
          }
          
          // Validar password si security no es "open"
          if (data.security !== 'open' && !data.password) {
            HostBerry.showAlert('warning', t('hostapd.password_required', 'Password is required for WPA2/WPA3 security'));
            if (submitBtn) {
              submitBtn.disabled = false;
              submitBtn.innerHTML = originalBtnText;
            }
            return;
          }
          
          try {
            const resp = await HostBerry.apiRequest('/api/v1/hostapd/config', {
              method: 'POST',
              body: data
            });
            
            if (resp && resp.ok) {
              const result = await resp.json();
              if (result.error) {
                HostBerry.showAlert('warning', translateError(result.error));
              } else {
                HostBerry.showAlert('success', t('hostapd.config_saved', 'HostAPD configuration saved successfully. The service will be restarted automatically.'));
                // Recargar estado después de guardar
                setTimeout(() => {
                  loadHostAPDStatus();
                  loadAccessPoints();
                  loadClients();
                }, 1500);
              }
            } else {
              const errorText = await resp.text().catch(() => '');
              const errorMsg = translateError(errorText) || t('errors.configuration_error', 'Configuration error');
              HostBerry.showAlert('danger', errorMsg);
            }
          } catch (e) {
            console.error('Error saving HostAPD config:', e);
            HostBerry.showAlert('danger', t('errors.network_error', 'Network error') + ': ' + (e.message || String(e)));
          } finally {
            // Restaurar botón
            if (submitBtn) {
              submitBtn.disabled = false;
              submitBtn.innerHTML = originalBtnText;
            }
          }
        });
      }
      
      // Actualizar periódicamente
      setInterval(() => {
        loadHostAPDStatus();
        loadAccessPoints();
        loadClients();
      }, 30000);
    });
  }

  // Inicializar cuando el DOM esté listo
  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', initHostAPDPage);
  } else {
    initHostAPDPage();
  }
})();
