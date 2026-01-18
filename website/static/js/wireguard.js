// JS extraído desde templates/wireguard.html
(function(){
  async function loadConfig(){
    try{
      const resp = await fetch('/api/v1/wireguard/config', { headers:{ 'Authorization': `Bearer ${localStorage.getItem('access_token')}` } });
      if(resp.ok){
        const data = await resp.json();
        const ta = document.getElementById('wg_config');
        if(ta && data && typeof data.config === 'string'){
          ta.value = data.config;
        }
      }
    }catch(e){ console.error('Error loading wireguard config:', e); }
  }

  async function loadInterfaces(){
    try{
      const resp = await fetch('/api/v1/wireguard/interfaces', { headers:{ 'Authorization': `Bearer ${localStorage.getItem('access_token')}` } });
      if(resp.ok){
        const interfaces = await resp.json();
        const tbody = document.getElementById('interfacesTable'); if(!tbody) return;
        tbody.innerHTML = '';
        interfaces.forEach(function(iface){
          const tr = document.createElement('tr');
          tr.innerHTML = '<td>'+iface.name+'</td>'+
            '<td><span class="badge bg-'+(iface.status==='up'?'success':'danger')+'">'+iface.status+'</span></td>'+
            '<td>'+iface.address+'</td>'+
            '<td>'+iface.peers_count+'</td>'+
            '<td><button class="btn btn-sm btn-outline-primary" onclick="configureInterface(\''+iface.name+'\')"><i class="bi bi-gear"></i></button></td>';
          tbody.appendChild(tr);
        });
      }
    }catch(e){ console.error('Error loading interfaces:', e); }
  }

  async function loadPeers(){
    try{
      const resp = await fetch('/api/v1/wireguard/peers', { headers:{ 'Authorization': `Bearer ${localStorage.getItem('access_token')}` } });
      if(resp.ok){
        const peers = await resp.json();
        const tbody = document.getElementById('peersTable'); if(!tbody) return;
        tbody.innerHTML = '';
        peers.forEach(function(peer){
          const tr = document.createElement('tr');
          tr.innerHTML = '<td>'+peer.name+'</td>'+
            '<td><span class="badge bg-'+(peer.connected?'success':'danger')+'">'+(peer.connected?(HostBerry.t?HostBerry.t('wireguard.connected','Connected'):'Connected'):(HostBerry.t?HostBerry.t('wireguard.disconnected','Disconnected'):'Disconnected'))+'</span></td>'+
            '<td>'+peer.bandwidth+'</td>'+
            '<td>'+peer.uptime+'</td>';
          tbody.appendChild(tr);
        });
      }
    }catch(e){ console.error('Error loading peers:', e); }
  }

  async function toggleWireGuard(){
    try{ const resp = await fetch('/api/v1/wireguard/toggle', { method:'POST', headers:{ 'Authorization': `Bearer ${localStorage.getItem('access_token')}` } });
      if(resp.ok){ HostBerry.showAlert('success', HostBerry.t('messages.operation_successful')); setTimeout(()=>window.location.reload(), 1000); }
      else { HostBerry.showAlert('danger', HostBerry.t('errors.operation_failed')); }
    }catch(_e){ HostBerry.showAlert('danger', HostBerry.t('errors.network_error')); }
  }

  async function restartWireGuard(){
    try{ const resp = await fetch('/api/v1/wireguard/restart', { method:'POST', headers:{ 'Authorization': `Bearer ${localStorage.getItem('access_token')}` } });
      if(resp.ok){ HostBerry.showAlert('success', HostBerry.t('messages.operation_successful')); setTimeout(()=>window.location.reload(), 2000); }
      else { HostBerry.showAlert('danger', HostBerry.t('errors.operation_failed')); }
    }catch(_e){ HostBerry.showAlert('danger', HostBerry.t('errors.network_error')); }
  }

  function configureInterface(name){ alert(HostBerry.t('wireguard.configuring_interface','Configurando interfaz')+': '+name); }

  const cfgForm = document.getElementById('wireguardConfigForm');
  if(cfgForm){
    cfgForm.addEventListener('submit', async function(e){
      e.preventDefault();
      const fd = new FormData(this);
      const data = { config: fd.get('config') };
      try{
        const resp = await fetch('/api/v1/wireguard/config', { method:'POST', headers:{ 'Content-Type':'application/json', 'Authorization': `Bearer ${localStorage.getItem('access_token')}` }, body: JSON.stringify(data) });
        if(resp.ok){ HostBerry.showAlert('success', HostBerry.t('messages.changes_saved')); }
        else { HostBerry.showAlert('danger', HostBerry.t('errors.configuration_error')); }
      }catch(_e){ HostBerry.showAlert('danger', HostBerry.t('errors.network_error')); }
    });
  }

  document.addEventListener('DOMContentLoaded', function(){
    loadConfig();
    loadInterfaces(); loadPeers(); setInterval(function(){ loadInterfaces(); loadPeers(); }, 30000);
  });

  window.toggleWireGuard = toggleWireGuard;
  window.restartWireGuard = restartWireGuard;
  window.configureInterface = configureInterface;
})();

