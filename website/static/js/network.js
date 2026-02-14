// Network Management JavaScript
(function(){
  // Esperar a que HostBerry esté disponible
  function waitForHostBerry(callback, maxAttempts = 50) {
    if (window.HostBerry && window.HostBerry.apiRequest && window.HostBerry.t) {
      callback();
    } else if (maxAttempts > 0) {
      setTimeout(() => waitForHostBerry(callback, maxAttempts - 1), 100);
    } else {
      console.error('HostBerry not available after waiting');
      callback();
    }
  }

  function t(key, fallback) {
    return window.HostBerry?.t ? window.HostBerry.t(key, fallback) : (fallback || key);
  }

  function escapeHtml(value) {
    const s = String(value ?? '');
    return window.HostBerry?.escapeHtml
      ? window.HostBerry.escapeHtml(s)
      : s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;');
  }

  // DNS Presets
  const dnsPresets = {
    google: { primary: '8.8.8.8', secondary: '8.8.4.4' },
    cloudflare: { primary: '1.1.1.1', secondary: '1.0.0.1' },
    opendns: { primary: '208.67.222.222', secondary: '208.67.220.220' },
    quad9: { primary: '9.9.9.9', secondary: '149.112.112.112' },
    adguard: { primary: '94.140.14.14', secondary: '94.140.15.15' }
  };

  // Apply DNS Preset
  window.applyDNSPreset = function() {
    const preset = document.getElementById('dns-preset')?.value;
    if (preset && preset !== 'custom' && dnsPresets[preset]) {
      const dns = dnsPresets[preset];
      const dns1 = document.getElementById('dns1');
      const dns2 = document.getElementById('dns2');
      if (dns1) dns1.value = dns.primary;
      if (dns2) dns2.value = dns.secondary;
    }
  };


  // Load Network Interfaces
  async function loadInterfaces() {
    const container = document.getElementById('interfacesContainer');
    if (container) {
      container.innerHTML = '<div class="text-center py-4"><div class="spinning mb-3"><i class="bi bi-arrow-clockwise" style="font-size: 2rem;"></i></div><p class="text-muted">' + t('network.loading_interfaces', 'Loading interfaces...') + '</p></div>';
    }
    
    try {
      const resp = await HostBerry.apiRequest('/api/v1/network/interfaces');
      if (!resp) {
        throw new Error('No response from server');
      }
      
      if (resp && resp.ok) {
        let data;
        try {
          data = await resp.json();
        } catch (jsonError) {
          console.error('Error parsing JSON:', jsonError);
          if (container) {
            container.innerHTML = '<p class="text-muted text-center">' + t('network.no_interfaces', 'No interfaces found') + '</p>';
          }
          return;
        }
        
        // Manejar diferentes formatos de respuesta
        let interfaces = [];
        if (Array.isArray(data)) {
          interfaces = data;
        } else if (data.interfaces && Array.isArray(data.interfaces)) {
          interfaces = data.interfaces;
        } else if (data.data && Array.isArray(data.data)) {
          interfaces = data.data;
        } else if (data.success !== false && data.interfaces) {
          interfaces = Array.isArray(data.interfaces) ? data.interfaces : [];
        }
        
        if (interfaces.length > 0) {
          displayInterfaces(interfaces);
          // Extraer nombres de interfaces para el selector
          const interfaceNames = interfaces.map(iface => {
            if (typeof iface === 'string') return iface;
            return iface.name || iface.interface || '';
          }).filter(name => name && name !== 'lo');
          populateInterfaceSelects(interfaceNames);
        } else {
          // Actualizar contadores cuando no hay interfaces
          const activeCountEl = document.getElementById('active-interfaces-count');
          if (activeCountEl) {
            activeCountEl.textContent = '0';
          }
          updateNetworkStatusCard([]);
          if (container) {
            container.innerHTML = '<p class="text-muted text-center">' + t('network.no_interfaces', 'No interfaces found') + '</p>';
          }
        }
      } else {
        // Solo mostrar error si es un error real (500, etc), no si es 200 con datos vacíos
        const status = resp.status;
        if (status >= 500) {
          const errorText = await resp.text().catch(() => '');
          console.error('Server error loading interfaces:', status, errorText);
          HostBerry.showAlert('danger', t('errors.loading_interfaces', 'Error loading interfaces'));
        }
        if (container) {
          container.innerHTML = '<p class="text-muted text-center">' + t('network.no_interfaces', 'No interfaces found') + '</p>';
        }
      }
    } catch (e) {
      console.error('Exception loading interfaces:', e);
      // Solo mostrar notificación si es un error de red real, no un error de parsing
      if (e.message && !e.message.includes('JSON')) {
        HostBerry.showAlert('danger', t('errors.network_error', 'Network connection error'));
      }
      if (container) {
        container.innerHTML = '<p class="text-muted text-center">' + t('network.no_interfaces', 'No interfaces found') + '</p>';
      }
    }
  }

  // Actualizar badge de interfaz en el Network Traffic Chart
  function updateTrafficInterfaceBadge(interfaceName) {
    const badge = document.getElementById('net-traffic-interface-badge');
    if (!badge) return;
    
    if (interfaceName && interfaceName !== '') {
      badge.textContent = interfaceName;
      badge.classList.remove('bg-secondary');
      badge.classList.add('bg-primary');
    } else {
      badge.textContent = t('monitoring.network_auto', 'Auto');
      badge.classList.remove('bg-primary');
      badge.classList.add('bg-secondary');
    }
  }

  function displayInterfaces(interfaces) {
    const container = document.getElementById('interfacesContainer');
    if (!container) return;
    
    if (!interfaces || !Array.isArray(interfaces) || interfaces.length === 0) {
      container.innerHTML = '<p class="text-muted text-center">' + t('network.no_interfaces', 'No interfaces found') + '</p>';
      return;
    }
    
    let html = '<div class="table-responsive"><table class="table table-hover"><thead><tr><th>' + t('network.interface', 'Interface') + '</th><th>' + t('network.status', 'Status') + '</th><th>' + t('network.ip_address', 'IP Address') + '</th><th>' + t('network.mac_address', 'MAC Address') + '</th></tr></thead><tbody>';
    
    let activeCount = 0;
    interfaces.forEach(function(iface) {
      if (!iface) return;
      // Verificar estado de la interfaz - puede venir como status, state, o connected
      const statusValue = iface.status || iface.state || '';
      const isAPMode = iface.ap_mode === true || iface.ap_mode === 'true';
      const hasInternet = iface.internet_connected === true || iface.internet_connected === 'true';
      
      // Determinar estado real
      let isUp = false;
      let statusClass = 'danger';
      let statusIcon = 'bi-x-circle';
      let statusText = t('network.disconnected', 'Disconnected');
      
      if (isAPMode) {
        // Modo AP - no tiene conexión a Internet
        isUp = true; // La interfaz está activa pero en modo AP
        statusClass = 'warning';
        statusIcon = 'bi-broadcast';
        statusText = t('network.ap_mode', 'AP Mode (No Internet)');
      } else if (statusValue === 'ap_mode') {
        // También detectar cuando el estado es explícitamente 'ap_mode'
        isUp = true;
        statusClass = 'warning';
        statusIcon = 'bi-broadcast';
        statusText = t('network.ap_mode', 'AP Mode (No Internet)');
      } else if (statusValue === 'up' || statusValue === 'connected' || 
                 iface.connected === true || 
                 (statusValue && statusValue.toLowerCase() === 'up') ||
                 (iface.ip && iface.ip !== 'N/A' && iface.ip !== '')) {
        isUp = true;
        if (hasInternet) {
          // Conectado y tiene Internet
          statusClass = 'success';
          statusIcon = 'bi-check-circle';
          statusText = t('network.connected', 'Connected');
        } else {
          // Conectado pero sin Internet
          statusClass = 'warning';
          statusIcon = 'bi-exclamation-triangle';
          statusText = t('network.no_internet', 'No Internet');
        }
      }
      
      if (isUp && !isAPMode && hasInternet) activeCount++;
      const ifaceName = iface.name || iface.interface || 'Unknown';
      const ifaceIp = iface.ip || iface.ip_address || 'N/A';
      const ifaceMac = iface.mac || iface.mac_address || 'N/A';
      const safeIfaceName = escapeHtml(ifaceName);
      const safeIfaceIp = escapeHtml(ifaceIp);
      const safeIfaceMac = escapeHtml(ifaceMac);
      const safeStatusText = escapeHtml(statusText);
      
      html += '<tr><td><strong>' + safeIfaceName + '</strong></td><td><span class="badge bg-' + statusClass + '"><i class="bi ' + statusIcon + '"></i> ' + safeStatusText + '</span></td><td>' + safeIfaceIp + '</td><td>' + safeIfaceMac + '</td></tr>';
    });
    
    html += '</tbody></table></div>';
    container.innerHTML = html;
    
    // Update active interfaces count
    const activeCountEl = document.getElementById('active-interfaces-count');
    if (activeCountEl) {
      activeCountEl.textContent = activeCount.toString();
    }
    
    // También actualizar el estado de la red con las interfaces
    updateNetworkStatusCard(interfaces);
  }

  function populateInterfaceSelects(interfaces) {
    const selects = [
      document.getElementById('net-traffic-interface-select')
    ];
    
    selects.forEach(select => {
      if (!select) return;
      
      // Guardar el valor seleccionado actual
      const currentValue = select.value;
      
      // Clear existing options except first
      while (select.options.length > 1) {
        select.remove(1);
      }
      
      if (Array.isArray(interfaces) && interfaces.length > 0) {
        interfaces.forEach(iface => {
          const ifaceName = (typeof iface === 'string') ? iface : (iface.name || iface.interface || iface);
          if (ifaceName && ifaceName !== 'lo' && ifaceName !== '') {
            const option = document.createElement('option');
            option.value = ifaceName;
            option.textContent = ifaceName;
            select.appendChild(option);
          }
        });
        
        // Restaurar el valor seleccionado si todavía existe
        if (currentValue) {
          const optionExists = Array.from(select.options).some(opt => opt.value === currentValue);
          if (optionExists) {
            select.value = currentValue;
          }
        }
      }
    });
  }

  // Load Routing Table
  async function loadRoutingTable() {
    const tbody = document.getElementById('routingTable');
    if (tbody) {
      tbody.innerHTML = '<tr><td colspan="4" class="text-center text-muted"><div class="spinning mb-2"><i class="bi bi-arrow-clockwise"></i></div><p class="mb-0">' + t('network.loading_routes', 'Loading routes...') + '</p></td></tr>';
    }
    
    try {
      const resp = await HostBerry.apiRequest('/api/v1/network/routing');
      if (!resp) {
        throw new Error('No response from server');
      }
      
      if (resp && resp.ok) {
        let data;
        try {
          const text = await resp.text();
          data = JSON.parse(text);
        } catch (jsonError) {
          console.error('Error parsing JSON:', jsonError);
          if (tbody) {
            tbody.innerHTML = '<tr><td colspan="4" class="text-center text-muted">' + t('network.no_routes', 'No routes found') + '</td></tr>';
          }
          return;
        }
        
        // Manejar diferentes formatos de respuesta
        let routes = [];
        if (Array.isArray(data)) {
          routes = data;
        } else if (data.routes && Array.isArray(data.routes)) {
          routes = data.routes;
        } else if (data.data && Array.isArray(data.data)) {
          routes = data.data;
        }
        
        if (routes.length > 0) {
          displayRoutingTable(routes);
        } else {
          if (tbody) {
            tbody.innerHTML = '<tr><td colspan="4" class="text-center text-muted">' + t('network.no_routes', 'No routes found') + '</td></tr>';
          }
        }
      } else {
        // Solo mostrar error si es un error real del servidor (500, etc)
        const status = resp.status;
        if (status >= 500) {
          const errorText = await resp.text().catch(() => '');
          console.error('Server error loading routing table:', status, errorText);
          HostBerry.showAlert('danger', t('errors.loading_routing', 'Error loading routing table'));
        }
        if (tbody) {
          tbody.innerHTML = '<tr><td colspan="4" class="text-center text-muted">' + t('network.no_routes', 'No routes found') + '</td></tr>';
        }
      }
    } catch (e) {
      console.error('Exception loading routing table:', e);
      // Solo mostrar notificación si es un error de red real, no un error de parsing
      if (e.message && !e.message.includes('JSON')) {
        HostBerry.showAlert('danger', t('errors.network_error', 'Network connection error'));
      }
      if (tbody) {
        tbody.innerHTML = '<tr><td colspan="4" class="text-center text-muted">' + t('network.no_routes', 'No routes found') + '</td></tr>';
      }
    }
  }

  function displayRoutingTable(routes) {
    const tbody = document.getElementById('routingTable');
    if (!tbody) return;
    
    if (!routes || !Array.isArray(routes) || routes.length === 0) {
      tbody.innerHTML = '<tr><td colspan="4" class="text-center text-muted">' + t('network.no_routes', 'No routes found') + '</td></tr>';
      return;
    }
    
    let html = '';
    routes.forEach(function(route) {
      if (!route) return;
      const safeDestination = escapeHtml(route.destination || '0.0.0.0');
      const safeGateway = escapeHtml(route.gateway || '*');
      const safeInterface = escapeHtml(route.interface || route.dev || '-');
      const safeMetric = escapeHtml(route.metric || '0');
      html += '<tr><td>' + safeDestination + '</td><td>' + safeGateway + '</td><td>' + safeInterface + '</td><td>' + safeMetric + '</td></tr>';
    });
    tbody.innerHTML = html;
  }

  // Network Traffic Monitoring
  let selectedTrafficInterface = '';
  let lastTrafficSnapshot = null;
  let netTrafficChart = null;
  const netTrafficHistory = { labels: [], download: [], upload: [] };

  function formatBytes(bytes) {
    if (HostBerry && HostBerry.formatBytes) return HostBerry.formatBytes(bytes, 1);
    if (!bytes || !Number.isFinite(bytes) || bytes < 0) return '0 B';
    const k = 1024, sizes = ['B', 'KB', 'MB', 'GB', 'TB'];
    const i = Math.max(0, Math.min(Math.floor(Math.log(bytes) / Math.log(k)), sizes.length - 1));
    return (bytes / Math.pow(k, i)).toFixed(1) + ' ' + sizes[i];
  }

  function computeNetworkRates(current) {
    if (!current || typeof current !== 'object') {
      console.warn('computeNetworkRates: invalid current data', current);
      return {
        download_speed: 0.0,
        upload_speed: 0.0,
        bytes_recv: 0,
        bytes_sent: 0,
        interface: selectedTrafficInterface || '',
        ip_address: '--'
      };
    }
    
    const bytesRecv = typeof current.bytes_recv === 'number' && !isNaN(current.bytes_recv) ? current.bytes_recv : 0;
    const bytesSent = typeof current.bytes_sent === 'number' && !isNaN(current.bytes_sent) ? current.bytes_sent : 0;
    
    if (lastTrafficSnapshot && current.interface && lastTrafficSnapshot.interface && current.interface !== lastTrafficSnapshot.interface) {
      lastTrafficSnapshot = null;
      netTrafficHistory.labels.length = 0;
      netTrafficHistory.download.length = 0;
      netTrafficHistory.upload.length = 0;
      if (netTrafficChart) netTrafficChart.update();
    }
    
    const now = Date.now();
    if (!lastTrafficSnapshot) {
      lastTrafficSnapshot = { 
        time: now, 
        bytes_recv: bytesRecv,
        bytes_sent: bytesSent,
        interface: current.interface || selectedTrafficInterface || ''
      };
      return {
        ...current,
        download_speed: 0.0,
        upload_speed: 0.0,
        bytes_recv: bytesRecv,
        bytes_sent: bytesSent
      };
    }
    
    const timeDelta = (now - lastTrafficSnapshot.time) / 1000;
    if (timeDelta <= 0.1 || timeDelta > 60) {
      lastTrafficSnapshot = { 
        time: now, 
        bytes_recv: bytesRecv,
        bytes_sent: bytesSent,
        interface: current.interface || selectedTrafficInterface || ''
      };
      return {
        ...current,
        download_speed: 0.0,
        upload_speed: 0.0,
        bytes_recv: bytesRecv,
        bytes_sent: bytesSent
      };
    }
    
    const prevBytesRecv = lastTrafficSnapshot.bytes_recv || 0;
    const prevBytesSent = lastTrafficSnapshot.bytes_sent || 0;
    
    let bytesRecvDelta = bytesRecv - prevBytesRecv;
    let bytesSentDelta = bytesSent - prevBytesSent;
    
    if (bytesRecvDelta < 0 && Math.abs(bytesRecvDelta) > prevBytesRecv * 0.5) {
      bytesRecvDelta = bytesRecv;
    }
    if (bytesSentDelta < 0 && Math.abs(bytesSentDelta) > prevBytesSent * 0.5) {
      bytesSentDelta = bytesSent;
    }
    
    const downloadSpeed = Math.max(0, bytesRecvDelta / timeDelta);
    const uploadSpeed = Math.max(0, bytesSentDelta / timeDelta);
    
    lastTrafficSnapshot = {
      time: now,
      bytes_recv: bytesRecv,
      bytes_sent: bytesSent,
      interface: current.interface || selectedTrafficInterface || ''
    };
    
    return {
      ...current,
      download_speed: downloadSpeed,
      upload_speed: uploadSpeed,
      bytes_recv: bytesRecv,
      bytes_sent: bytesSent
    };
  }

  function pushTrafficHistory(download, upload) {
    const downloadValue = (typeof download === 'number' && !isNaN(download) && isFinite(download)) ? download : 0;
    const uploadValue = (typeof upload === 'number' && !isNaN(upload) && isFinite(upload)) ? upload : 0;
    
    const now = new Date();
    const timeLabel = now.getHours().toString().padStart(2, '0') + ':' + 
                      now.getMinutes().toString().padStart(2, '0') + ':' + 
                      now.getSeconds().toString().padStart(2, '0');
    
    netTrafficHistory.labels.push(timeLabel);
    netTrafficHistory.download.push(Math.max(0, downloadValue));
    netTrafficHistory.upload.push(Math.max(0, uploadValue));
    
    if (netTrafficHistory.labels.length > 30) {
      netTrafficHistory.labels.shift();
      netTrafficHistory.download.shift();
      netTrafficHistory.upload.shift();
    }
  }

  function ensureTrafficChart() {
    const canvas = document.getElementById('net-traffic-chart');
    if (!canvas) {
      // Intentar de nuevo después de un tiempo si el canvas no está disponible
      setTimeout(ensureTrafficChart, 500);
      return;
    }
    
    if (netTrafficChart) return;
    
    // Verificar si Chart.js está disponible
    if (typeof Chart === 'undefined' && typeof window.Chart === 'undefined') {
      console.warn('Chart.js not loaded, retrying...');
      setTimeout(ensureTrafficChart, 500);
      return;
    }
    
    const ChartLib = typeof Chart !== 'undefined' ? Chart : window.Chart;
    
    if (netTrafficHistory.labels.length === 0) {
      const now = new Date();
      for (let i = 9; i >= 0; i--) {
        const time = new Date(now.getTime() - i * 1000);
        const timeLabel = time.getHours().toString().padStart(2, '0') + ':' + 
                          time.getMinutes().toString().padStart(2, '0') + ':' + 
                          time.getSeconds().toString().padStart(2, '0');
        netTrafficHistory.labels.push(timeLabel);
        netTrafficHistory.download.push(0);
        netTrafficHistory.upload.push(0);
      }
    }
    
    try {
      const ctx = canvas.getContext('2d');
      // Ocultar mensaje de carga
      const chartLoading = document.getElementById('chart-loading');
      if (chartLoading) chartLoading.style.display = 'none';
      
      netTrafficChart = new ChartLib(ctx, {
        type: 'line',
        data: {
          labels: netTrafficHistory.labels,
          datasets: [
            {
              label: t('monitoring.download', 'Download'),
              data: netTrafficHistory.download,
              borderColor: '#0dcaf0',
              backgroundColor: 'rgba(13, 202, 240, 0.1)',
              tension: 0.4,
              fill: true,
              pointRadius: 0,
              pointHoverRadius: 4
            },
            {
              label: t('monitoring.upload', 'Upload'),
              data: netTrafficHistory.upload,
              borderColor: '#198754',
              backgroundColor: 'rgba(25, 135, 84, 0.1)',
              tension: 0.4,
              fill: true,
              pointRadius: 0,
              pointHoverRadius: 4
            }
          ]
        },
        options: {
          responsive: true,
          maintainAspectRatio: false,
          animation: { duration: 0 },
          plugins: {
            legend: {
              labels: { color: window.getComputedStyle(document.body).color },
              display: true
            },
            tooltip: {
              mode: 'index',
              intersect: false
            }
          },
          scales: {
            x: { 
              ticks: { 
                color: window.getComputedStyle(document.body).color,
                maxRotation: 45,
                minRotation: 0
              },
              grid: { color: 'rgba(255,255,255,0.1)' }
            },
            y: { 
              ticks: { 
                color: window.getComputedStyle(document.body).color,
                callback: function(value) {
                  return formatBytes(value) + '/s';
                }
              },
              grid: { color: 'rgba(255,255,255,0.1)' },
              beginAtZero: true
            }
          },
          interaction: {
            mode: 'nearest',
            axis: 'x',
            intersect: false
          }
        }
      });
    } catch (error) {
      console.error('Error creating traffic chart:', error);
    }
  }

  // Parse /proc/net/dev format
  function parseProcNetDev(raw, interfaceName) {
    if (!raw || typeof raw !== 'string') {
      return null;
    }
    
    const lines = raw.split('\n');
    let bytesRecv = 0;
    let bytesSent = 0;
    let packetsRecv = 0;
    let packetsSent = 0;
    let errors = 0;
    let drop = 0;
    let foundInterface = null;
    const interfaces = [];
    
    for (const line of lines) {
      const trimmed = line.trim();
      // Filtrar líneas de encabezado y vacías
      if (!trimmed || 
          trimmed.startsWith('Inter-') || 
          trimmed.startsWith(' face') ||
          trimmed.startsWith('face') ||
          trimmed.toLowerCase().includes('receive') ||
          trimmed.toLowerCase().includes('transmit') ||
          trimmed === 'face' ||
          trimmed.match(/^\s*$/)) {
        continue;
      }
      
      // Formato: eth0: 12345 67890 123 456 789 012 345 678 901 234 567 890
      // O también puede ser: wlan0:    12345    67890    123    456    789    012    345    678    901    234    567    890
      // Campos: name, bytes_recv, packets_recv, errs_recv, drop_recv, bytes_sent, packets_sent, errs_sent, drop_sent
      // Usar split con regex para manejar múltiples espacios
      const parts = trimmed.split(/\s+/).filter(p => p.length > 0);
      if (parts.length < 10) {
        console.warn('Line has insufficient parts:', trimmed, 'parts:', parts.length);
        continue;
      }
      
      // El primer elemento es el nombre de la interfaz con dos puntos
      const ifaceName = parts[0].replace(':', '').trim();
      // Filtrar interfaces inválidas
      if (!ifaceName || 
          ifaceName === 'lo' || 
          ifaceName === 'face' ||
          ifaceName.toLowerCase() === 'interface' ||
          ifaceName.length > 20 || // Nombres de interfaz no deberían ser tan largos
          !ifaceName.match(/^[a-zA-Z0-9_-]+$/)) { // Solo caracteres alfanuméricos, guiones y guiones bajos
        continue;
      }
      
      // Los campos están en posiciones específicas después del nombre
      // parts[0] = nombre, parts[1-8] = receive stats, parts[9-16] = transmit stats
      const recvBytes = parseInt(parts[1], 10) || 0;
      const recvPackets = parseInt(parts[2], 10) || 0;
      const recvErrs = parseInt(parts[3], 10) || 0;
      const recvDrop = parseInt(parts[4], 10) || 0;
      const sentBytes = parseInt(parts[9], 10) || 0;
      const sentPackets = parseInt(parts[10], 10) || 0;
      const sentErrs = parseInt(parts[11], 10) || 0;
      const sentDrop = parseInt(parts[12], 10) || 0;
      
      interfaces.push(ifaceName);
      
      // Si se especificó una interfaz, solo usar esa
      if (interfaceName && ifaceName === interfaceName) {
        foundInterface = ifaceName;
        bytesRecv = recvBytes;
        bytesSent = sentBytes;
        packetsRecv = recvPackets;
        packetsSent = sentPackets;
        errors = recvErrs + sentErrs;
        drop = recvDrop + sentDrop;
        break; // Salir del loop cuando se encuentra la interfaz especificada
      } else if (!interfaceName) {
        // Si no se especificó interfaz, usar la primera no-loopback con tráfico
        if ((recvBytes > 0 || sentBytes > 0) && !foundInterface) {
          foundInterface = ifaceName;
          bytesRecv = recvBytes;
          bytesSent = sentBytes;
          packetsRecv = recvPackets;
          packetsSent = sentPackets;
          errors = recvErrs + sentErrs;
          drop = recvDrop + sentDrop;
        }
      }
    }
    
    // Si se especificó una interfaz pero no se encontró, devolver 0 pero con el nombre de la interfaz
    if (interfaceName && !foundInterface) {
      // La interfaz especificada no existe en /proc/net/dev
      console.warn(`⚠ Interface ${interfaceName} not found in /proc/net/dev. Available interfaces:`, interfaces);
      console.warn('Raw data sample (first 500 chars):', raw.substring(0, 500));
      // Devolver datos vacíos pero con el nombre de la interfaz para que el frontend sepa qué interfaz se está mostrando
      return {
        bytes_recv: 0,
        bytes_sent: 0,
        packets_recv: 0,
        packets_sent: 0,
        errors: 0,
        drop: 0,
        interface: interfaceName,
        interfaces: interfaces
      };
    }
    
    // Si no se especificó interfaz y no se encontró ninguna con tráfico, usar la primera disponible
    if (!foundInterface && interfaces.length > 0 && !interfaceName) {
      foundInterface = interfaces[0];
      // Necesitamos leer los datos de nuevo para esta interfaz
      for (const line of lines) {
        const trimmed = line.trim();
        if (!trimmed || 
            trimmed.startsWith('Inter-') || 
            trimmed.startsWith(' face') ||
            trimmed.startsWith('face') ||
            trimmed.toLowerCase().includes('receive') ||
            trimmed.toLowerCase().includes('transmit') ||
            trimmed === 'face') continue;
        const parts = trimmed.split(/\s+/);
        if (parts.length < 10) continue;
        const ifaceName = parts[0].replace(':', '').trim();
        if (!ifaceName || ifaceName === 'face' || ifaceName === 'lo') continue;
        if (ifaceName === foundInterface) {
          bytesRecv = parseInt(parts[1]) || 0;
          bytesSent = parseInt(parts[9]) || 0;
          packetsRecv = parseInt(parts[2]) || 0;
          packetsSent = parseInt(parts[10]) || 0;
          errors = (parseInt(parts[3]) || 0) + (parseInt(parts[11]) || 0);
          drop = (parseInt(parts[4]) || 0) + (parseInt(parts[12]) || 0);
          break;
        }
      }
    }
    
    return {
      bytes_recv: bytesRecv,
      bytes_sent: bytesSent,
      packets_recv: packetsRecv,
      packets_sent: packetsSent,
      errors: errors,
      drop: drop,
      interface: foundInterface || '',
      interfaces: interfaces
    };
  }

  window.updateTrafficStats = async function() {
    try {
      let url = '/api/v1/system/network';
      if (selectedTrafficInterface) {
        url += `?interface=${encodeURIComponent(selectedTrafficInterface)}&_t=${Date.now()}`;
      } else {
        url += `?_t=${Date.now()}`;
      }
      const resp = await HostBerry.apiRequest(url);
      
      if (!resp || !resp.ok) {
        throw new Error('Network request failed');
      }
      
      const networkStatsRaw = await resp.json();
      if (!networkStatsRaw || typeof networkStatsRaw !== 'object') {
        throw new Error('Invalid network stats response');
      }
      
      // Si la respuesta tiene "raw", parsearla
      let parsedData = networkStatsRaw;
      if (networkStatsRaw.raw && typeof networkStatsRaw.raw === 'string') {
        parsedData = parseProcNetDev(networkStatsRaw.raw, selectedTrafficInterface);
        if (!parsedData) {
          console.warn('Failed to parse /proc/net/dev');
          parsedData = networkStatsRaw;
        }
      }
      
      const networkStats = computeNetworkRates(parsedData);
      
      // Poblar selector de interfaces
      if (parsedData.interfaces && Array.isArray(parsedData.interfaces) && parsedData.interfaces.length > 0) {
        populateInterfaceSelects(parsedData.interfaces);
      } else if (networkStats.interfaces && Array.isArray(networkStats.interfaces)) {
        populateInterfaceSelects(networkStats.interfaces);
      } else if (networkStatsRaw.interfaces && Array.isArray(networkStatsRaw.interfaces)) {
        populateInterfaceSelects(networkStatsRaw.interfaces);
      }
      
      // Actualizar badge con la interfaz que realmente se está usando
      const currentInterface = networkStats.interface || networkStatsRaw.interface || selectedTrafficInterface || '';
      updateTrafficInterfaceBadge(currentInterface);

      const setText = (id, value) => {
        const el = document.getElementById(id);
        if (el) el.textContent = value;
      };

      setText('net-traffic-interface', networkStats.interface || networkStatsRaw.interface || '--');
      setText('net-traffic-ip', networkStats.ip_address || networkStatsRaw.ip_address || '--');
      
      const downloadSpeed = networkStats.download_speed || 0;
      const uploadSpeed = networkStats.upload_speed || 0;
      
      const downloadText = downloadSpeed > 0 ? formatBytes(downloadSpeed) + '/s' : '0 B/s';
      const uploadText = uploadSpeed > 0 ? formatBytes(uploadSpeed) + '/s' : '0 B/s';
      
      setText('net-traffic-download', downloadText);
      setText('net-traffic-upload', uploadText);
      setText('traffic-download-value', downloadText);
      setText('traffic-upload-value', uploadText);
      
      const bytesRecv = networkStats.bytes_recv || networkStatsRaw.bytes_recv || 0;
      const bytesSent = networkStats.bytes_sent || networkStatsRaw.bytes_sent || 0;
      setText('net-traffic-bytes-recv', formatBytes(bytesRecv));
      setText('net-traffic-bytes-sent', formatBytes(bytesSent));
      
      const packetsSent = networkStats.packets_sent || networkStatsRaw.packets_sent || 0;
      const packetsRecv = networkStats.packets_recv || networkStatsRaw.packets_recv || 0;
      setText('net-traffic-packets', `${packetsSent} / ${packetsRecv}`);
      
      const errors = (networkStats.errors || networkStatsRaw.errors || 0) + 
                     (networkStats.drop || networkStatsRaw.drop || 0);
      setText('net-traffic-errors', errors || '0');

      const validDownloadSpeed = (typeof downloadSpeed === 'number' && !isNaN(downloadSpeed) && isFinite(downloadSpeed)) ? downloadSpeed : 0;
      const validUploadSpeed = (typeof uploadSpeed === 'number' && !isNaN(uploadSpeed) && isFinite(uploadSpeed)) ? uploadSpeed : 0;
      
      pushTrafficHistory(validDownloadSpeed, validUploadSpeed);

      ensureTrafficChart();
      
      if (netTrafficChart) {
        netTrafficChart.data.labels = [...netTrafficHistory.labels];
        netTrafficChart.data.datasets[0].data = [...netTrafficHistory.download];
        netTrafficChart.data.datasets[1].data = [...netTrafficHistory.upload];
        netTrafficChart.data.datasets[0].label = t('monitoring.download', 'Download');
        netTrafficChart.data.datasets[1].label = t('monitoring.upload', 'Upload');
        netTrafficChart.update('none');
      }
    } catch (error) {
      console.error('Error updating traffic stats:', error);
    }
  };

  // Load Network Status - usa los datos de interfaces ya cargados
  function updateNetworkStatusCard(interfaces) {
    const statusEl = document.getElementById('network-status-value');
    const statusBar = document.getElementById('network-status-bar');
    
    if (!statusEl) return;
    
    if (!interfaces || !Array.isArray(interfaces) || interfaces.length === 0) {
      statusEl.textContent = t('network.disconnected', 'Disconnected');
      if (statusBar) statusBar.style.width = '0%';
      return;
    }
    
    // Contar interfaces activas con conexión real a Internet (no modo AP)
    const activeCount = interfaces.filter(iface => {
      if (!iface) return false;
      const isAPMode = iface.ap_mode === true || iface.ap_mode === 'true' || iface.state === 'ap_mode';
      const hasInternet = iface.internet_connected === true || iface.internet_connected === 'true';
      const status = iface.status || iface.state || '';
      const hasIp = iface.ip && iface.ip !== 'N/A' && iface.ip !== '';
      // Solo contar si está conectado, tiene IP, no está en modo AP, y tiene Internet
      return (status === 'up' || status === 'connected') && hasIp && !isAPMode && hasInternet;
    }).length;
    
    if (activeCount > 0) {
      statusEl.textContent = t('network.connected', 'Connected');
      const percent = Math.min(100, Math.round((activeCount / interfaces.length) * 100));
      if (statusBar) statusBar.style.width = percent + '%';
    } else {
      statusEl.textContent = t('network.disconnected', 'Disconnected');
      if (statusBar) statusBar.style.width = '0%';
    }
  }

  // Load Network Basic Configuration
  async function loadNetworkConfig() {
    try {
      // Obtener configuración actual desde el endpoint
      const configResp = await HostBerry.apiRequest('/api/v1/network/config');
      if (!configResp || !configResp.ok) {
        console.warn('No se pudo obtener la configuración de red');
        return;
      }
      
      const config = await configResp.json().catch(() => ({}));
      
      // Actualizar campos del formulario
      const hostnameInput = document.getElementById('hostname');
      const gatewayInput = document.getElementById('gateway');
      const dns1Input = document.getElementById('dns1');
      const dns2Input = document.getElementById('dns2');
      
      if (hostnameInput && config.hostname) {
        hostnameInput.value = config.hostname;
      }
      if (gatewayInput && config.gateway) {
        gatewayInput.value = config.gateway;
      }
      if (dns1Input && config.dns1) {
        dns1Input.value = config.dns1;
      }
      if (dns2Input && config.dns2) {
        dns2Input.value = config.dns2;
      }
      
      // Actualizar preset de DNS si coincide con alguno conocido
      if (config.dns1 && config.dns2) {
        const presetSelect = document.getElementById('dns-preset');
        if (presetSelect) {
          let matchedPreset = 'custom';
          for (const [presetKey, presetValue] of Object.entries(dnsPresets)) {
            if (presetValue.primary === config.dns1 && presetValue.secondary === config.dns2) {
              matchedPreset = presetKey;
              break;
            }
          }
          presetSelect.value = matchedPreset;
        }
      }
    } catch (e) {
      console.error('Error loading network config:', e);
    }
  }

  // LibreSpeed speed test
  async function runSpeedtest() {
    const runBtn = document.getElementById('speedtest-run-btn');
    const resultDiv = document.getElementById('speedtest-result');
    const progressDiv = document.getElementById('speedtest-progress');
    const errorDiv = document.getElementById('speedtest-error');
    if (!runBtn || !resultDiv || !progressDiv || !errorDiv) return;
    runBtn.disabled = true;
    resultDiv.classList.add('d-none');
    errorDiv.classList.add('d-none');
    errorDiv.textContent = '';
    progressDiv.classList.remove('d-none');
    try {
      const resp = await HostBerry.apiRequest('/api/v1/network/speedtest', { method: 'POST' });
      const data = await resp.json().catch(() => ({}));
      if (resp.ok && data.success === true) {
        const fmt = (n) => (n == null || n === '' ? '—' : Number(n).toLocaleString(undefined, { maximumFractionDigits: 2 }));
        document.getElementById('speedtest-ping').textContent = data.ping_ms != null ? fmt(data.ping_ms) + ' ms' : '—';
        document.getElementById('speedtest-jitter').textContent = data.jitter_ms != null ? fmt(data.jitter_ms) + ' ms' : '—';
        document.getElementById('speedtest-download').textContent = data.download_mbps != null ? fmt(data.download_mbps) + ' Mbps' : '—';
        document.getElementById('speedtest-upload').textContent = data.upload_mbps != null ? fmt(data.upload_mbps) + ' Mbps' : '—';
        const serverEl = document.getElementById('speedtest-server');
        const clientEl = document.getElementById('speedtest-client');
        if (serverEl) serverEl.textContent = data.server_name ? (t('network.speedtest_server', 'Server') + ': ' + data.server_name) : '';
        if (clientEl) clientEl.textContent = data.client_ip || data.client_org ? (data.client_ip || '') + (data.client_org ? ' — ' + data.client_org : '') : '';
        resultDiv.classList.remove('d-none');
      } else {
        errorDiv.textContent = data.error || t('network.speedtest_error', 'Speed test failed');
        errorDiv.classList.remove('d-none');
      }
    } catch (e) {
      errorDiv.textContent = t('network.speedtest_error', 'Speed test failed') + ': ' + (e.message || String(e));
      errorDiv.classList.remove('d-none');
    } finally {
      progressDiv.classList.add('d-none');
      runBtn.disabled = false;
    }
  }

  // Initialize Network Page
  function initNetworkPage() {
    loadInterfaces();
    loadRoutingTable();
    loadNetworkConfig(); // Cargar configuración actual
    
    // Inicializar badge de interfaz
    updateTrafficInterfaceBadge(selectedTrafficInterface || '');
    
    // Network Basic Config Form
    const basicConfigForm = document.getElementById('networkBasicConfigForm');
    if (basicConfigForm) {
      basicConfigForm.addEventListener('submit', async function(e) {
        e.preventDefault();
        
        // Obtener el botón de submit
        const submitButton = this.querySelector('button[type="submit"]');
        const originalButtonText = submitButton ? submitButton.innerHTML : '';
        const originalButtonDisabled = submitButton ? submitButton.disabled : false;
        
        // Mostrar spinner y deshabilitar botón
        if (submitButton) {
          submitButton.disabled = true;
          submitButton.innerHTML = '<span class="spinning me-2"><i class="bi bi-arrow-clockwise"></i></span>' + 
                                   t('network.saving', 'Saving...');
        }
        
        const fd = new FormData(this);
        const data = {
          hostname: fd.get('hostname'),
          dns1: fd.get('dns1'),
          dns2: fd.get('dns2'),
          gateway: fd.get('gateway')
        };
        try {
          const resp = await HostBerry.apiRequest('/api/v1/network/config', {
            method: 'POST',
            body: data
          });
          if (resp && resp.ok) {
            const result = await resp.json();
            if (result.success !== false) {
              HostBerry.showAlert('success', result.message || t('messages.config_saved', 'Configuration saved'));
              // Recargar configuración después de guardar
              setTimeout(() => loadNetworkConfig(), 1000);
            } else {
              const message = result.message || result.error || t('errors.config_update_error', 'Error updating configuration');
              HostBerry.showAlert('warning', message);
            }
          } else {
            const errorText = await resp.text().catch(() => '');
            HostBerry.showAlert('danger', t('errors.config_update_error', 'Error updating configuration') + (errorText ? ': ' + errorText : ''));
          }
        } catch (e) {
          console.error('Error saving network config:', e);
          HostBerry.showAlert('danger', t('errors.network_error', 'Network error'));
        } finally {
          // Restaurar botón original
          if (submitButton) {
            submitButton.disabled = originalButtonDisabled;
            submitButton.innerHTML = originalButtonText;
          }
        }
      });
    }

    // Traffic Interface Select
    const trafficSelect = document.getElementById('net-traffic-interface-select');
    if (trafficSelect) {
      trafficSelect.addEventListener('change', function(e) {
        selectedTrafficInterface = e.target.value || '';
        lastTrafficSnapshot = null;
        netTrafficHistory.labels.length = 0;
        netTrafficHistory.download.length = 0;
        netTrafficHistory.upload.length = 0;
        if (netTrafficChart) netTrafficChart.update();
        // Actualizar badge con la interfaz seleccionada
        updateTrafficInterfaceBadge(selectedTrafficInterface);
        updateTrafficStats();
      });
    }
    
    // Start traffic updates
    updateTrafficStats();
    setInterval(updateTrafficStats, 5000);
    
    // Refresh interfaces and routing table periodically
    setInterval(() => {
      loadInterfaces();
      loadRoutingTable();
    }, 30000);
  }

  // Export functions
  window.loadInterfaces = loadInterfaces;
  window.loadRoutingTable = loadRoutingTable;
  window.runSpeedtest = runSpeedtest;

  // Initialize
  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', () => waitForHostBerry(initNetworkPage));
  } else {
    waitForHostBerry(initNetworkPage);
  }
})();
