// Monitoring dashboard interactions
(function(){
  const setText = (id, value) => {
    const el = document.getElementById(id);
    if(el) el.textContent = value;
  };

  function safeToFixed(value, digits = 1){
    if(typeof value !== 'number' || Number.isNaN(value)) return '0.0';
    return value.toFixed(digits);
  }

  const formatUptime = (s) => (HostBerry && HostBerry.formatUptime ? HostBerry.formatUptime(s) : (!Number.isFinite(s) || s < 0 ? '--' : `${Math.floor(s/86400)}d ${Math.floor((s%86400)/3600)}h ${Math.floor((s%3600)/60)}m`));
  function formatBytes(bytes) {
    if (HostBerry && HostBerry.formatBytes) return HostBerry.formatBytes(bytes, 1);
    if (!Number.isFinite(bytes)) return '0 MB';
    return `${safeToFixed(bytes / (1024 * 1024), 1)} MB`;
  }

  async function fetchJson(url){
    try {
      if(!HostBerry || !HostBerry.apiRequest){
        throw new Error('HostBerry.apiRequest is not available');
      }
      
      const resp = await HostBerry.apiRequest(url);
      if(!resp){
        throw new Error('No response from server');
      }
      
      if(resp.status === 401){
        // token inválido, redirige a login
        window.location.href = '/login';
        throw new Error('Unauthorized');
      }
      
      if(!resp.ok){
        let errText = '';
        try {
          errText = await resp.text();
        } catch(e) {
          errText = `HTTP ${resp.status} ${resp.statusText}`;
        }
        throw new Error(`Request failed ${resp.status}: ${errText}`);
      }
      
      const data = await resp.json();
      if(!data){
        throw new Error('Empty response from server');
      }
      
      return data;
    } catch (error) {
      console.error('Error en fetchJson:', url, error);
      throw error;
    }
  }

  function updateProgress(id, percent){
    const bar = document.getElementById(id);
    if(bar && Number.isFinite(percent)){
      const clamped = Math.min(100, Math.max(0, percent));
      bar.style.width = `${clamped}%`;
    }
  }

  let netChart;
  let selectedInterface = '';
  const netHistory = {
    labels: [],
    download: [],
    upload: []
  };
  let lastNetSnapshot = null;

  function ensureNetChart(){
    if(netChart) return netChart;
    const ctx = document.getElementById('net-chart');
    if(!ctx || typeof Chart === 'undefined') return null;
    netChart = new Chart(ctx, {
      type: 'line',
      data: {
        labels: netHistory.labels,
        datasets: [
          {
            label: (window.HostBerry && window.HostBerry.t ? window.HostBerry.t('monitoring.download_kb_s', 'Download KB/s') : 'Download KB/s'),
            data: netHistory.download,
            borderColor: '#6366f1',
            backgroundColor: 'rgba(99, 102, 241, 0.25)',
            tension: 0.25,
            fill: true
          },
          {
            label: (window.HostBerry && window.HostBerry.t ? window.HostBerry.t('monitoring.upload_kb_s', 'Upload KB/s') : 'Upload KB/s'),
            data: netHistory.upload,
            borderColor: '#ec4899',
            backgroundColor: 'rgba(236, 72, 153, 0.2)',
            tension: 0.25,
            fill: true
          }
        ]
      },
      options: {
        responsive: true,
        scales: {
          y: { beginAtZero: true, ticks: { color: '#cbd5e1' }, grid: { color: 'rgba(255,255,255,0.08)' } },
          x: { ticks: { color: '#cbd5e1' }, grid: { display: false } }
        },
        plugins: { legend: { labels: { color: '#e2e8f0' } } }
      }
    });
    return netChart;
  }

  function pushNetHistory(download, upload){
    const now = new Date().toLocaleTimeString();
    netHistory.labels.push(now);
    netHistory.download.push(download);
    netHistory.upload.push(upload);
    const maxPoints = 20;
    if(netHistory.labels.length > maxPoints){
      netHistory.labels.shift();
      netHistory.download.shift();
      netHistory.upload.shift();
    }
    if(netChart){
      netChart.update();
    }
  }

  async function updateStats(){
    try{
      if(updateStats._retryCount) updateStats._retryCount = 0;
      const [systemStatsResp, networkStatsResp] = await Promise.all([
        fetchJson('/api/v1/system/stats'),
        fetchJson(`/api/v1/system/network${selectedInterface ? `?interface=${encodeURIComponent(selectedInterface)}` : ''}`)
      ]);
      
      // Manejar respuesta de system stats (puede venir directamente o envuelta)
      let systemStats = systemStatsResp;
      if(systemStatsResp && typeof systemStatsResp === 'object'){
        // Si tiene 'data', usarlo; si no, usar directamente
        systemStats = systemStatsResp.data || systemStatsResp;
      }
      
      if(!systemStats || typeof systemStats !== 'object'){
        console.error('Invalid system stats response:', systemStatsResp);
        throw new Error('Invalid system stats response');
      }
      
      // Manejar respuesta de network stats (puede venir directamente o envuelta)
      let networkStatsRaw = networkStatsResp;
      if(networkStatsResp && typeof networkStatsResp === 'object'){
        // Si tiene 'data', usarlo; si no, usar directamente
        networkStatsRaw = networkStatsResp.data || networkStatsResp;
      }
      
      if(!networkStatsRaw || typeof networkStatsRaw !== 'object'){
        console.error('Invalid network stats response:', networkStatsResp);
        throw new Error('Invalid network stats response');
      }
      
      const networkStats = computeNetworkRates(networkStatsRaw);
      
      // Populate interface select
      if(networkStats.interfaces && Array.isArray(networkStats.interfaces)){
        populateInterfaceSelect(networkStats.interfaces);
      } else if(networkStatsRaw.interfaces && Array.isArray(networkStatsRaw.interfaces)){
        populateInterfaceSelect(networkStatsRaw.interfaces);
      }

      // Actualizar uptime
      const uptime = systemStats.uptime || systemStats.uptime_seconds || 0;
      setText('uptime-value', formatUptime(uptime));
      
      // Actualizar CPU
      const cpuUsage = systemStats.cpu_usage || systemStats.cpu_percent || 0;
      const cpuUsageValue = typeof cpuUsage === 'number' ? cpuUsage : parseFloat(cpuUsage) || 0;
      setText('cpu-usage', `${safeToFixed(cpuUsageValue)}%`);
      updateProgress('cpu-progress', cpuUsageValue);
      
      const cpuTemp = systemStats.cpu_temperature || systemStats.temperature;
      if(typeof cpuTemp === 'number' && cpuTemp > 0){
        setText('cpu-temp', `${safeToFixed(cpuTemp)}°C`);
      } else {
        setText('cpu-temp', '--°C');
      }
      
      const cpuCores = systemStats.cpu_cores || systemStats.cpu_count;
      setText('cpu-cores', cpuCores ? String(cpuCores) : '-');

      // Actualizar Memoria
      const memUsage = systemStats.memory_usage || systemStats.memory_percent || 0;
      const memUsageValue = typeof memUsage === 'number' ? memUsage : parseFloat(memUsage) || 0;
      setText('mem-usage', `${safeToFixed(memUsageValue)}%`);
      updateProgress('mem-progress', memUsageValue);
      
      const memTotal = systemStats.memory_total || systemStats.memory_total_bytes;
      if(memTotal && memTotal > 0){
        const memTotalGB = typeof memTotal === 'number' ? memTotal : parseFloat(memTotal) || 0;
        setText('mem-total', `${safeToFixed(memTotalGB / (1024 ** 3),1)} GB`);
      } else {
        setText('mem-total', '0 GB');
      }
      
      const memFree = systemStats.memory_free || systemStats.memory_available;
      if(memFree && memFree > 0){
        const memFreeGB = typeof memFree === 'number' ? memFree : parseFloat(memFree) || 0;
        setText('mem-free', `${safeToFixed(memFreeGB / (1024 ** 3),1)} GB`);
      } else {
        setText('mem-free', '0 GB');
      }

      // Actualizar Disco
      const diskUsage = systemStats.disk_usage || systemStats.disk_percent || 0;
      const diskUsageValue = typeof diskUsage === 'number' ? diskUsage : parseFloat(diskUsage) || 0;
      setText('disk-usage', `${safeToFixed(diskUsageValue)}%`);
      updateProgress('disk-progress', diskUsageValue);
      
      const diskTotal = systemStats.disk_total || systemStats.disk_total_bytes;
      if(diskTotal && diskTotal > 0){
        const diskTotalGB = typeof diskTotal === 'number' ? diskTotal : parseFloat(diskTotal) || 0;
        setText('disk-total', `${safeToFixed(diskTotalGB / (1024 ** 3),1)} GB`);
      } else {
        setText('disk-total', '0 GB');
      }
      
      const diskUsed = systemStats.disk_used || systemStats.disk_used_bytes;
      if(diskUsed && diskUsed > 0){
        const diskUsedGB = typeof diskUsed === 'number' ? diskUsed : parseFloat(diskUsed) || 0;
        setText('disk-used', `${safeToFixed(diskUsedGB / (1024 ** 3),1)} GB`);
      } else {
        setText('disk-used', '0 GB');
      }

      // Actualizar Red
      setText('net-interface', networkStats.interface || networkStatsRaw.interface || '-');
      setText('net-ip', networkStats.ip_address || networkStatsRaw.ip_address || '--');
      
      const downloadSpeed = networkStats.download_speed || 0;
      const uploadSpeed = networkStats.upload_speed || 0;
      setText('net-download', `${safeToFixed(downloadSpeed, 2)} KB/s`);
      setText('net-upload', `${safeToFixed(uploadSpeed, 2)} KB/s`);
      
      const bytesRecv = networkStats.bytes_recv || networkStatsRaw.bytes_recv || 0;
      const bytesSent = networkStats.bytes_sent || networkStatsRaw.bytes_sent || 0;
      setText('net-bytes-recv', formatBytes(bytesRecv));
      setText('net-bytes-sent', formatBytes(bytesSent));
      
      const packetsSent = networkStats.packets_sent || networkStatsRaw.packets_sent || 0;
      const packetsRecv = networkStats.packets_recv || networkStatsRaw.packets_recv || 0;
      setText('net-packets', `${packetsSent} / ${packetsRecv}`);
      
      pushNetHistory(downloadSpeed, uploadSpeed);

      setText('monitoring-last-update', new Date().toLocaleTimeString());
      ensureNetChart();
    }catch(error){
      console.error('Error updating monitoring stats:', error);
      console.error('Error details:', error.message, error.stack);
      
      // Mostrar error más descriptivo
      let errorMsg = 'Unable to refresh monitoring stats';
      if(error.message){
        errorMsg += ': ' + error.message;
      }
      
      // Intentar usar traducción si está disponible
      if(HostBerry && HostBerry.t){
        const translated = HostBerry.t('errors.monitoring_stats', errorMsg);
        if(translated && translated !== 'errors.monitoring_stats'){
          errorMsg = translated;
        }
      }
      
      // Mostrar alerta si está disponible, sino usar console
      if(HostBerry && HostBerry.showAlert){
        HostBerry.showAlert('danger', errorMsg);
      } else {
        console.error(errorMsg);
      }
      
      // Establecer valores por defecto para evitar mostrar 0s
      setText('uptime-value', '--');
      setText('cpu-usage', '--');
      setText('mem-usage', '--');
      setText('disk-usage', '--');
      setText('net-download', '--');
      setText('net-upload', '--');
      
      // Reintentar hasta 3 veces (intervalo 5 s)
      if(typeof updateStats._retryCount !== 'number') updateStats._retryCount = 0;
      if(updateStats._retryCount < 3){
        updateStats._retryCount++;
        setTimeout(() => updateStats(), 5000);
      } else {
        updateStats._retryCount = 0;
      }
    }
  }

  function computeNetworkRates(current){
    // Si no hay datos, devolver objeto con valores por defecto
    if(!current || typeof current !== 'object'){
      return {
        download_speed: 0.0,
        upload_speed: 0.0,
        bytes_recv: 0,
        bytes_sent: 0,
        packets_sent: 0,
        packets_recv: 0,
        interface: selectedInterface || '',
        ip_address: '--',
        interfaces: []
      };
    }
    
    // Asegurar que tenemos bytes_recv y bytes_sent como números
    const bytesRecv = typeof current.bytes_recv === 'number' && !isNaN(current.bytes_recv) ? current.bytes_recv : 0;
    const bytesSent = typeof current.bytes_sent === 'number' && !isNaN(current.bytes_sent) ? current.bytes_sent : 0;
    
    // reset snapshot if cambia interfaz
    if(lastNetSnapshot && current.interface && lastNetSnapshot.interface && current.interface !== lastNetSnapshot.interface){
      lastNetSnapshot = null;
      netHistory.labels.length = 0;
      netHistory.download.length = 0;
      netHistory.upload.length = 0;
      if(netChart){ netChart.update(); }
    }
    
    const now = Date.now();
    if(!lastNetSnapshot){
      lastNetSnapshot = { 
        time: now, 
        bytes_recv: bytesRecv,
        bytes_sent: bytesSent,
        interface: current.interface || selectedInterface || ''
      };
      // Primera vez, no hay velocidad calculable
      return {
        ...current,
        download_speed: 0.0,
        upload_speed: 0.0,
        bytes_recv: bytesRecv,
        bytes_sent: bytesSent
      };
    }
    
    const elapsedSec = (now - lastNetSnapshot.time) / 1000;
    if(elapsedSec <= 0 || elapsedSec > 300 || !Number.isFinite(elapsedSec)){
      // Si pasó mucho tiempo o tiempo inválido, resetear snapshot
      lastNetSnapshot = { 
        time: now, 
        bytes_recv: bytesRecv,
        bytes_sent: bytesSent,
        interface: current.interface || selectedInterface || ''
      };
      return {
        ...current,
        download_speed: 0.0,
        upload_speed: 0.0,
        bytes_recv: bytesRecv,
        bytes_sent: bytesSent
      };
    }
    
    // Calcular velocidades en KB/s
    const prevBytesRecv = typeof lastNetSnapshot.bytes_recv === 'number' ? lastNetSnapshot.bytes_recv : 0;
    const prevBytesSent = typeof lastNetSnapshot.bytes_sent === 'number' ? lastNetSnapshot.bytes_sent : 0;
    const dlRate = (bytesRecv - prevBytesRecv) / 1024 / elapsedSec;
    const ulRate = (bytesSent - prevBytesSent) / 1024 / elapsedSec;
    
    lastNetSnapshot = { 
      time: now, 
      bytes_recv: bytesRecv,
      bytes_sent: bytesSent,
      interface: current.interface || selectedInterface || ''
    };
    
    return {
      ...current,
      download_speed: Math.max(0, Number.isFinite(dlRate) ? dlRate : 0),
      upload_speed: Math.max(0, Number.isFinite(ulRate) ? ulRate : 0),
      bytes_recv: bytesRecv,
      bytes_sent: bytesSent
    };
  }

  function initMonitoring(){
    updateStats();
    setInterval(updateStats, 60000);
    const ifaceSelect = document.getElementById('net-interface-select');
    if(ifaceSelect){
      ifaceSelect.addEventListener('change', ()=>{
        selectedInterface = ifaceSelect.value;
        lastNetSnapshot = null;
        netHistory.labels.length = 0;
        netHistory.download.length = 0;
        netHistory.upload.length = 0;
        if(netChart){ netChart.update(); }
        updateStats();
      });
    }
  }

  document.addEventListener('DOMContentLoaded', initMonitoring);

  function populateInterfaceSelect(list){
    const select = document.getElementById('net-interface-select');
    if(!select || !Array.isArray(list)) return;
    const current = select.value;
    const existing = Array.from(select.options).map(o => o.value);
    let changed = false;
    list.forEach(iface=>{
      if(iface && !existing.includes(iface)){
        const opt = document.createElement('option');
        opt.value = iface;
        opt.textContent = iface;
        select.appendChild(opt);
        changed = true;
      }
    });
    if(current && !list.includes(current)){
      select.value = '';
      selectedInterface = '';
      changed = true;
    }
    if(!current && selectedInterface){
      select.value = selectedInterface;
    }
    if(changed && select.value !== current){
      select.dispatchEvent(new Event('change'));
    }
  }
})();
