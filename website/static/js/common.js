// Common JS for simple UI behaviours (offline)
(function(){
  // Global Namespace
  const HostBerry = window.HostBerry || {};

  async function performLogout(event){
    if(event){ event.preventDefault(); }
    try{
      await HostBerry.apiRequest('/api/v1/auth/logout', { method: 'POST' });
    }catch(_e){
      // ignore errors to ensure client-side logout proceeds
    }finally{
      localStorage.removeItem('access_token');
      localStorage.removeItem('user_info');
      if(HostBerry.showAlert){
        HostBerry.showAlert('info', HostBerry.t ? HostBerry.t('auth.logout_success', 'Logout successful') : 'Logout successful');
      }
      window.location.href = '/login';
    }
  }

  async function performRestart(event){
    if(event){ event.preventDefault(); }
    const confirmMsg = HostBerry.t ? HostBerry.t('system.restart_confirm', 'Are you sure you want to restart the system? This will disconnect all users.') : 'Are you sure you want to restart the system? This will disconnect all users.';
    if(!confirm(confirmMsg)) return;
    
    try{
      if(HostBerry.showAlert){
        HostBerry.showAlert('warning', HostBerry.t ? HostBerry.t('system.restarting', 'Restarting system...') : 'Restarting system...');
      }
      await HostBerry.apiRequest('/api/v1/system/restart', { method: 'POST' });
      if(HostBerry.showAlert){
        HostBerry.showAlert('info', HostBerry.t ? HostBerry.t('system.restart_pending', 'Restart command sent') : 'Restart command sent');
      }
      setTimeout(function(){ window.location.reload(); }, 5000);
    }catch(error){
      console.error('Restart failed', error);
      if(HostBerry.showAlert){
        HostBerry.showAlert('danger', HostBerry.t ? HostBerry.t('system.restart_error', 'Unable to restart system') : 'Unable to restart system');
      }
    }
  }

  async function performShutdown(event){
    if(event){ event.preventDefault(); }
    const confirmMsg = HostBerry.t ? HostBerry.t('system.shutdown_confirm', 'Are you sure you want to shutdown the system? This will disconnect all users.') : 'Are you sure you want to shutdown the system? This will disconnect all users.';
    if(!confirm(confirmMsg)) return;
    
    const doubleConfirm = HostBerry.t ? HostBerry.t('system.shutdown_double_confirm', 'Type SHUTDOWN to confirm') : 'Type SHUTDOWN to confirm';
    const userInput = prompt(doubleConfirm);
    if(userInput !== 'SHUTDOWN') return;
    
    try{
      if(HostBerry.showAlert){
        HostBerry.showAlert('warning', HostBerry.t ? HostBerry.t('system.shutting_down', 'Shutting down system...') : 'Shutting down system...');
      }
      await HostBerry.apiRequest('/api/v1/system/shutdown', { method: 'POST' });
      if(HostBerry.showAlert){
        HostBerry.showAlert('info', HostBerry.t ? HostBerry.t('system.shutdown_pending', 'Shutdown command sent') : 'Shutdown command sent');
      }
    }catch(error){
      console.error('Shutdown failed', error);
      if(HostBerry.showAlert){
        HostBerry.showAlert('danger', HostBerry.t ? HostBerry.t('system.shutdown_error', 'Unable to shutdown system') : 'Unable to shutdown system');
      }
    }
  }

  // Simple Dropdown without Bootstrap
  document.addEventListener('click', function(e){
    const toggle = e.target.closest('.dropdown-toggle');
    const inDropdown = e.target.closest('.dropdown');
    const isDropdownItem = e.target.closest('.dropdown-item');
    
    // Si se hace clic en un item del dropdown, no cerrar el dropdown
    if(isDropdownItem && !isDropdownItem.hasAttribute('data-action')){
      return;
    }
    
    document.querySelectorAll('.dropdown').forEach(function(d){
      if(!inDropdown || d !== inDropdown) d.classList.remove('show');
    });
    if(toggle){
      e.preventDefault();
      e.stopPropagation();
      const dd = toggle.closest('.dropdown');
      if(dd) dd.classList.toggle('show');
    }
  });

  // Navbar Toggler (Hamburger)
  document.addEventListener('click', function(e){
    const toggler = e.target.closest('.navbar-toggler');
    if(toggler){
        const targetId = toggler.getAttribute('data-bs-target');
        if(targetId){
            const target = document.querySelector(targetId);
            if(target){
                target.classList.toggle('show');
                const expanded = target.classList.contains('show');
                toggler.setAttribute('aria-expanded', expanded);
            }
        }
    }
  });

  // Load embedded JSON translations if they exist
  function loadTranslations(){
    try{
      const el = document.getElementById('i18n-json');
      if(!el) return {};
      return JSON.parse(el.textContent || el.innerText || '{}');
    }catch(_e){
      return {};
    }
  }

  const translations = loadTranslations();

  // t: nested access to keys 'a.b.c'
  function t(key, defaultValue){
    if(!key) return defaultValue || '';
    const parts = String(key).split('.');
    let cur = translations;
    for(const part of parts){
      if(cur && Object.prototype.hasOwnProperty.call(cur, part)){
        cur = cur[part];
      }else{
        return defaultValue || key;
      }
    }
    return typeof cur === 'string' ? cur : (defaultValue || key);
  }

  // Floating alert top right
  function showAlert(type, message){
    const containerId = 'hb-alert-container';
    let container = document.getElementById(containerId);
    if(!container){
      container = document.createElement('div');
      container.id = containerId;
      container.style.position = 'fixed';
      container.style.top = '20px';
      container.style.right = '20px';
      container.style.zIndex = '9999';
      container.style.maxWidth = '360px';
      document.body.appendChild(container);
    }
    const alert = document.createElement('div');
    alert.className = 'alert alert-' + (type || 'info') + ' shadow';
    alert.style.marginBottom = '10px';
    alert.innerText = message || '';
    container.appendChild(alert);
    setTimeout(function(){
      if(alert && alert.parentNode){ alert.parentNode.removeChild(alert); }
    }, 5000);
  }

  // Fetch wrapper with JSON and token
  async function apiRequest(url, options){
    const opts = Object.assign({ method: 'GET', headers: {} }, options || {});
    const headers = new Headers(opts.headers);
    
    // Auth token
    if(!headers.has('Authorization')){
      let token = localStorage.getItem('access_token');
      // Fallback: si no hay token en localStorage, intentar leerlo de la URL (?token=)
      // Esto permite que funcione incluso si el navegador no guarda cookies.
      if(!token){
        try{
          const u = new URL(window.location.href);
          token = u.searchParams.get('token') || '';
        }catch(_e){
          token = '';
        }
      }
      if(token) headers.set('Authorization', 'Bearer ' + token);
    }
    
    // JSON body handling
    if(opts.body && typeof opts.body === 'object' && !(opts.body instanceof FormData)){
      if(!headers.has('Content-Type')){
        headers.set('Content-Type', 'application/json');
      }
      opts.body = JSON.stringify(opts.body);
    }
    
    opts.headers = headers;
    try {
    const resp = await fetch(url, opts);
      if(resp.status === 401 && !url.includes('/auth/login')){
        // Auto logout on unauthorized
        // Pero NO redirigir inmediatamente si es una operación que puede causar pérdida temporal de conexión
        // o si es un error de red (no un error real de autenticación)
        const isNetworkOperation = url.includes('/wifi/connect') || 
                                   url.includes('/network/') || 
                                   url.includes('/system/network');
        
        if(isNetworkOperation){
          console.warn('401 durante operación de red - puede ser pérdida temporal de conexión');
          // No redirigir inmediatamente, dejar que el código de manejo de errores lo haga
          // después de verificar si es un error real o temporal
        } else {
          // Solo cerrar sesión si NO es un error de red y es un 401 real
          // Verificar que realmente es un error de autenticación y no un error de red
          try {
            const errorData = await resp.clone().json().catch(() => ({}));
            const errorMsg = String(errorData.error || '');
            const errorCode = errorData.code || '';
            
            // Si es "Usuario no encontrado", puede ser un problema temporal - no cerrar sesión
            if(errorCode === 'USER_NOT_FOUND' || 
               errorMsg.toLowerCase().includes('usuario no encontrado') ||
               errorMsg.toLowerCase().includes('user not found')){
              console.warn('Usuario no encontrado durante operación - puede ser problema temporal');
              // No cerrar sesión, dejar que el código de manejo de errores específico lo maneje
              return resp;
            }
            
            // Si el mensaje indica que es un error de token/autenticación real, cerrar sesión
            if(errorMsg.includes('token') || errorMsg.includes('Token') || 
               errorMsg.includes('autorizado') || errorMsg.includes('authorized') ||
               errorMsg.includes('expirado') || errorMsg.includes('expired')){
              localStorage.removeItem('access_token');
              window.location.href = '/login?error=session_expired';
            }
          } catch(_e) {
            // Si no se puede parsear el error, asumir que es un error de autenticación real
            localStorage.removeItem('access_token');
            window.location.href = '/login?error=session_expired';
          }
        }
      }
    return resp;
    } catch (e) {
      console.error('API Request failed:', e);
      // No cerrar sesión por errores de red - podría ser temporal
      throw e;
    }
  }

  // Timezone/locale helpers (timezone guardado en Settings y servido por backend)
  function getServerTimezone(){
    return (window.HostBerryServerSettings && window.HostBerryServerSettings.timezone) ? window.HostBerryServerSettings.timezone : 'UTC';
  }

  function getServerLanguage(){
    const lang = (document.documentElement && document.documentElement.lang) ? document.documentElement.lang : 'en';
    return (lang === 'es') ? 'es' : 'en';
  }

  function formatTime(date, options){
    const tz = getServerTimezone();
    const lang = getServerLanguage();
    const locale = (lang === 'es') ? 'es-ES' : 'en-US';
    const d = (date instanceof Date) ? date : new Date(date);
    const fmtOpts = Object.assign({ hour12: false, hour: '2-digit', minute: '2-digit', second: '2-digit', timeZone: tz }, options || {});
    try{
      return new Intl.DateTimeFormat(locale, fmtOpts).format(d);
    }catch(_e){
      // Fallback si Intl/timeZone no está disponible
      return d.toLocaleTimeString(locale, { hour12: false, hour: '2-digit', minute: '2-digit', second: '2-digit' });
    }
  }

  function formatUptime(seconds){
    if (!Number.isFinite(seconds) || seconds < 0) return '--';
    const days = Math.floor(seconds / 86400);
    const hours = Math.floor((seconds % 86400) / 3600);
    const minutes = Math.floor((seconds % 3600) / 60);
    if (days > 0) return days + 'd ' + hours + 'h ' + minutes + 'm';
    if (hours > 0) return hours + 'h ' + minutes + 'm';
    return minutes + 'm';
  }

  function escapeHtml(text){
    if (text == null) return '';
    const div = document.createElement('div');
    div.textContent = String(text);
    return div.innerHTML;
  }

  function formatBytes(bytes, decimals){
    if (bytes === 0) return '0 B';
    if (!bytes || !Number.isFinite(bytes) || bytes < 0) return '0 B';
    const k = 1024;
    const dm = (decimals !== undefined && decimals >= 0) ? decimals : 2;
    const sizes = ['B', 'KB', 'MB', 'GB', 'TB'];
    const i = Math.floor(Math.log(bytes) / Math.log(k));
    const sizeIndex = Math.max(0, Math.min(i, sizes.length - 1));
    return parseFloat((bytes / Math.pow(k, sizeIndex)).toFixed(dm)) + ' ' + sizes[sizeIndex];
  }

  // Export
  window.t = t;
  HostBerry.t = t;
  HostBerry.showAlert = showAlert;
  HostBerry.apiRequest = apiRequest;
  HostBerry.getServerTimezone = getServerTimezone;
  HostBerry.formatTime = formatTime;
  HostBerry.formatUptime = formatUptime;
  HostBerry.formatBytes = formatBytes;
  HostBerry.escapeHtml = escapeHtml;

  // Verificar y mantener la sesión activa
  function setupSessionKeepAlive(){
    const token = localStorage.getItem('access_token');
    if(!token) return;
    
    let consecutiveErrors = 0;
    const maxConsecutiveErrors = 3; // Permitir hasta 3 errores consecutivos antes de cerrar sesión
    
    // Verificar token periódicamente para detectar expiración antes de que ocurra
    // Verificar cada 2 minutos (si el token expira en 1 hora, esto es seguro)
    setInterval(async function(){
      try{
        const token = localStorage.getItem('access_token');
        if(!token) return;
        
        // Hacer una petición simple para verificar si el token sigue válido
        const resp = await fetch('/api/v1/auth/me', {
          method: 'GET',
          headers: {
            'Authorization': 'Bearer ' + token
          }
        });
        
        if(resp && resp.status === 401){
          // Verificar que realmente es un error de autenticación y no de red
          try {
            const errorData = await resp.json().catch(() => ({}));
            const errorMsg = String(errorData.error || '').toLowerCase();
            const errorCode = errorData.code || '';
            
            // Si es "Usuario no encontrado", puede ser un problema temporal - no cerrar sesión
            if(errorCode === 'USER_NOT_FOUND' || 
               errorMsg.includes('usuario no encontrado') ||
               errorMsg.includes('user not found')){
              console.warn('Usuario no encontrado durante keep-alive - puede ser problema temporal');
              // Resetear contador ya que no es un error de autenticación real
              consecutiveErrors = 0;
              return;
            }
            
            // Solo cerrar sesión si es un error real de token/autenticación
            if(errorMsg.includes('token') || errorMsg.includes('expirado') || 
               errorMsg.includes('expired') || errorMsg.includes('invalid') ||
               errorMsg.includes('autorizado') || errorMsg.includes('authorized')){
              consecutiveErrors++;
              if(consecutiveErrors >= maxConsecutiveErrors){
                console.warn('Token expirado después de múltiples intentos, redirigiendo a login...');
                localStorage.removeItem('access_token');
                window.location.href = '/login?error=session_expired';
              }
            } else {
              // Resetear contador si no es un error de autenticación real
              consecutiveErrors = 0;
            }
          } catch(_e) {
            // Si no se puede parsear, podría ser un error de red - no cerrar sesión
            consecutiveErrors = 0;
          }
        } else if(resp && resp.ok){
          // Token válido, resetear contador de errores
          consecutiveErrors = 0;
        }
      }catch(e){
        // Ignorar errores de red - no cerrar sesión por problemas de conectividad
        // Solo incrementar contador si es un error que no sea de red
        if(e.message && !e.message.includes('Failed to fetch') && 
           !e.message.includes('NetworkError') &&
           !e.message.includes('ERR_INTERNET_DISCONNECTED') &&
           !e.message.includes('ERR_NETWORK_CHANGED')){
          consecutiveErrors++;
          if(consecutiveErrors >= maxConsecutiveErrors){
            console.warn('Múltiples errores consecutivos, verificando sesión...');
            // Intentar una última verificación antes de cerrar
            consecutiveErrors = 0; // Resetear para dar otra oportunidad
          }
        } else {
          // Es un error de red, resetear contador
          consecutiveErrors = 0;
        }
      }
    }, 2 * 60 * 1000); // Cada 2 minutos (verificar más frecuentemente para sesión de 1 hora)
  }

  function setupSidebarToggle(){
    const sidebarToggle = document.querySelector('.sidebar-toggle');
    const sidebar = document.querySelector('.sidebar-nav');
    if(!sidebarToggle || !sidebar) return;

    sidebarToggle.addEventListener('click', function(e){
      e.preventDefault();
      e.stopPropagation();
      sidebar.classList.toggle('show');
    });

    // Cerrar sidebar al hacer clic fuera en móviles
    document.addEventListener('click', function(e){
      if(window.innerWidth > 991) return;
      if(!sidebar.classList.contains('show')) return;
      if(sidebar.contains(e.target) || sidebarToggle.contains(e.target)) return;
      sidebar.classList.remove('show');
    }, { passive: true });
  }

  document.addEventListener('DOMContentLoaded', async function(){
    setupSidebarToggle();
    setupSessionKeepAlive();
    
    const el = document.getElementById('hb-current-username');
    const token = localStorage.getItem('access_token');
    if(el && token){
      try{
        const resp = await apiRequest('/api/v1/auth/me');
        if(resp && resp.ok){
          const data = await resp.json();
          if(data && data.username){
            el.textContent = data.username;
          }
        }
      }catch(_e){
        // silent
      }
    }

    document.querySelectorAll('[data-action="logout"]').forEach(function(btn){
      btn.addEventListener('click', performLogout);
    });
    
    document.querySelectorAll('[data-action="restart"]').forEach(function(btn){
      btn.addEventListener('click', performRestart);
    });
    
    document.querySelectorAll('[data-action="shutdown"]').forEach(function(btn){
      btn.addEventListener('click', performShutdown);
    });
  });

  // Compat: many views use showAlert() directly
  if(!window.showAlert){ window.showAlert = showAlert; }
  HostBerry.performLogout = performLogout;
  HostBerry.performRestart = performRestart;
  HostBerry.performShutdown = performShutdown;
  window.HostBerry = HostBerry;
})();
