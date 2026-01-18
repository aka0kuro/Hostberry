// JS extraído desde templates/vpn.html
(function(){
  async function loadConnections(){
    try{ const resp = await fetch('/api/v1/vpn/connections', { headers:{ 'Authorization': `Bearer ${localStorage.getItem('access_token')}` } });
      if(resp.ok){ const connections = await resp.json(); const tbody = document.getElementById('connectionsTable'); if(!tbody) return; tbody.innerHTML='';
        connections.forEach(function(conn){ const tr = document.createElement('tr'); tr.innerHTML = '<td>'+conn.name+'</td><td>'+conn.type+'</td>'+
        '<td><span class="badge bg-'+(conn.status==='connected'?'success':'danger')+'">'+(conn.status==='connected'?(HostBerry.t?HostBerry.t('vpn.connected','Connected'):'Connected'):(HostBerry.t?HostBerry.t('vpn.disconnected','Disconnected'):'Disconnected'))+'</span></td><td>'+conn.bandwidth+'</td>'+
        '<td><button class="btn btn-sm btn-outline-primary" onclick="toggleConnection(\''+conn.name+'\')"><i class="bi bi-'+(conn.status==='connected'?'pause':'play')+'"></i></button></td>'; tbody.appendChild(tr); }); }
    }catch(e){ console.error('Error loading connections:', e); }
  }
  async function loadServers(){
    try{ const resp = await fetch('/api/v1/vpn/servers', { headers:{ 'Authorization': `Bearer ${localStorage.getItem('access_token')}` } });
      if(resp.ok){ const servers = await resp.json(); const tbody = document.getElementById('serversTable'); if(!tbody) return; tbody.innerHTML='';
        servers.forEach(function(server){ const tr = document.createElement('tr'); tr.innerHTML = '<td>'+server.name+'</td><td>'+server.address+'</td>'+
        '<td><span class="badge bg-'+(server.status==='running'?'success':'danger')+'">'+server.status+'</span></td><td>'+server.clients_count+'</td>'; tbody.appendChild(tr); }); }
    }catch(e){ console.error('Error loading servers:', e); }
  }
  async function loadClients(){
    try{ const resp = await fetch('/api/v1/vpn/clients', { headers:{ 'Authorization': `Bearer ${localStorage.getItem('access_token')}` } });
      if(resp.ok){ const clients = await resp.json(); const tbody = document.getElementById('clientsTable'); if(!tbody) return; tbody.innerHTML='';
        clients.forEach(function(client){ const tr = document.createElement('tr'); tr.innerHTML = '<td>'+client.name+'</td><td>'+client.address+'</td>'+
        '<td><span class="badge bg-'+(client.connected?'success':'danger')+'">'+(client.connected?(HostBerry.t?HostBerry.t('vpn.connected','Connected'):'Connected'):(HostBerry.t?HostBerry.t('vpn.disconnected','Disconnected'):'Disconnected'))+'</span></td><td>'+client.bandwidth+'</td>'; tbody.appendChild(tr); }); }
    }catch(e){ console.error('Error loading clients:', e); }
  }
  async function toggleVPN(){ try{ const resp = await fetch('/api/v1/vpn/toggle', { method:'POST', headers:{ 'Authorization': `Bearer ${localStorage.getItem('access_token')}` } }); if(resp.ok){ HostBerry.showAlert('success', HostBerry.t('messages.operation_successful')); setTimeout(()=>window.location.reload(), 1000); } else { HostBerry.showAlert('danger', HostBerry.t('errors.operation_failed')); } }catch(_e){ HostBerry.showAlert('danger', HostBerry.t('errors.network_error')); } }
  async function connectVPN(){ try{ const resp = await fetch('/api/v1/vpn/connect', { method:'POST', headers:{ 'Authorization': `Bearer ${localStorage.getItem('access_token')}` } }); if(resp.ok){ HostBerry.showAlert('success', HostBerry.t('messages.operation_successful')); setTimeout(()=>window.location.reload(), 1000); } else { HostBerry.showAlert('danger', HostBerry.t('errors.operation_failed')); } }catch(_e){ HostBerry.showAlert('danger', HostBerry.t('errors.network_error')); } }
  async function toggleConnection(name){ try{ const resp = await fetch('/api/v1/vpn/connections/'+name+'/toggle', { method:'POST', headers:{ 'Authorization': `Bearer ${localStorage.getItem('access_token')}` } }); if(resp.ok){ HostBerry.showAlert('success', HostBerry.t('messages.operation_successful')); setTimeout(loadConnections, 1000); } else { HostBerry.showAlert('danger', HostBerry.t('errors.operation_failed')); } }catch(_e){ HostBerry.showAlert('danger', HostBerry.t('errors.network_error')); } }
  async function generateCertificates(){ try{ const resp = await fetch('/api/v1/vpn/certificates/generate', { method:'POST', headers:{ 'Authorization': `Bearer ${localStorage.getItem('access_token')}` } }); if(resp.ok){ HostBerry.showAlert('success', HostBerry.t('messages.operation_successful')); } else { HostBerry.showAlert('danger', HostBerry.t('errors.operation_failed')); } }catch(_e){ HostBerry.showAlert('danger', HostBerry.t('errors.network_error')); } }
  function viewSecurityLogs(){ alert(HostBerry.t('vpn.viewing_security_logs','Viendo logs de seguridad')); }
  const cfgForm = document.getElementById('vpnConfigForm');
  if(cfgForm){ cfgForm.addEventListener('submit', async function(e){ e.preventDefault(); const fd=new FormData(this); const data={ server_name:fd.get('server_name'), server_address:fd.get('server_address'), server_port:parseInt(fd.get('server_port')), protocol:fd.get('protocol'), encryption:fd.get('encryption') };
    try{ const resp = await fetch('/api/v1/vpn/config', { method:'POST', headers:{ 'Content-Type':'application/json', 'Authorization': `Bearer ${localStorage.getItem('access_token')}` }, body: JSON.stringify(data) }); if(resp.ok){ HostBerry.showAlert('success', HostBerry.t('messages.changes_saved')); } else { HostBerry.showAlert('danger', HostBerry.t('errors.configuration_error')); } }catch(_e){ HostBerry.showAlert('danger', HostBerry.t('errors.network_error')); }
  }); }
  document.addEventListener('DOMContentLoaded', function(){ loadConnections(); loadServers(); loadClients(); setInterval(function(){ loadConnections(); loadServers(); loadClients(); }, 30000); });
  window.toggleVPN = toggleVPN; window.connectVPN = connectVPN; window.toggleConnection = toggleConnection; window.generateCertificates = generateCertificates; window.viewSecurityLogs = viewSecurityLogs;
})();

