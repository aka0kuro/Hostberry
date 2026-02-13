// JS específico para login: toggles y alerts
(function(){
  // i18n desde dataset HTML (multiidioma)
  const __i18nEl = document.getElementById('i18n-data');
  const i18n = {
    user_not_found: __i18nEl ? __i18nEl.getAttribute('data-user-not-found') : 'User not found',
    incorrect_password: __i18nEl ? __i18nEl.getAttribute('data-incorrect-password') : 'Incorrect password',
    too_many_attempts: __i18nEl ? __i18nEl.getAttribute('data-too-many-attempts') : 'Too many attempts',
    connection_error: __i18nEl ? __i18nEl.getAttribute('data-connection-error') : 'Connection error',
    login_generic_error: __i18nEl ? __i18nEl.getAttribute('data-login-generic-error') : 'Login error',
    login_success: __i18nEl ? __i18nEl.getAttribute('data-login-success') : 'Login successful'
  };

  // Toggle mostrar/ocultar contraseña (usa emoji de ojo)
  (function(){
    const btn = document.getElementById('toggle-password');
    if(!btn) return;
    btn.addEventListener('click', function(){
      const input = document.getElementById('password');
      const eye = document.getElementById('eye-emoji');
      const isPass = input.getAttribute('type') === 'password';
      input.setAttribute('type', isPass ? 'text' : 'password');
      if(eye) eye.textContent = isPass ? '🙈' : '👁️';
      const hideText = window.HostBerry && window.HostBerry.t ? window.HostBerry.t('common.hide_password', 'Hide password') : 'Hide password';
      const showText = window.HostBerry && window.HostBerry.t ? window.HostBerry.t('common.show_password', 'Show password') : 'Show password';
      this.setAttribute('aria-label', isPass ? hideText : showText);
      this.setAttribute('title', isPass ? hideText : showText);
    });
  })();

  // Alert helper: usar HostBerry.showAlert si existe, si no fallback local (p. ej. login carga antes)
  const showAlert = (type, message) => {
    if (window.HostBerry && typeof window.HostBerry.showAlert === 'function') {
      window.HostBerry.showAlert(type, message);
    } else {
      const alertDiv = document.createElement('div');
      alertDiv.className = `alert alert-${type} alert-dismissible fade show position-fixed`;
      alertDiv.style.cssText = 'top:20px; right:20px; z-index:9999; min-width:300px;';
      alertDiv.innerHTML = String(message);
      document.body.appendChild(alertDiv);
      setTimeout(() => alertDiv.remove(), 5000);
    }
  };

  // Manejador del login
  (function(){
    const form = document.getElementById('loginForm');
    if(!form) return;
    form.addEventListener('submit', async function(e){
      e.preventDefault();
      const fd = new FormData(this);
      const data = {
        username: fd.get('username'),
        password: fd.get('password')
      };
      try{
        const resp = await fetch('/api/v1/auth/login', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify(data)
        });
        const result = await resp.json();
        if(resp.ok){
          // Guardar token en localStorage (para navegadores que no guardan cookies)
          localStorage.setItem('access_token', result.access_token);
          showAlert('success', i18n.login_success);
          setTimeout(()=>{
            // IMPORTANT: si no hay cookies, las páginas protegidas necesitan token por query (?token=)
            const token = encodeURIComponent(result.access_token || '');
            const lang = (document.documentElement && document.documentElement.lang === 'es') ? 'es' : 'en';
            const langQ = `lang=${encodeURIComponent(lang)}`;
            const tokenQ = token ? `token=${token}` : '';
            const qs = [langQ, tokenQ].filter(Boolean).join('&');
            if(result.password_change_required){
              window.location.href = `/first-login?${qs}`;
            } else {
              window.location.href = `/dashboard?${qs}`;
            }
          }, 800);
        } else {
          // Preferir mensaje real del backend si viene en {error: "..."}
          const backendError = (result && result.error) ? String(result.error) : '';
          if(resp.status === 422){
            showAlert('danger', backendError || i18n.login_generic_error);
            return;
          }
          if(resp.status === 404){
            showAlert('warning', backendError || i18n.user_not_found);
          } else if(resp.status === 401){
            // Puede ser contraseña incorrecta O ruta protegida por auth (token requerido)
            showAlert('danger', backendError || i18n.incorrect_password);
          } else if(resp.status === 429){
            showAlert('warning', backendError || i18n.too_many_attempts);
          } else {
            showAlert('danger', backendError || i18n.login_generic_error);
          }
        }
      } catch(err){
        console.error(err);
        showAlert('danger', i18n.connection_error);
      }
    });
  })();
})();

