// JS extraído desde templates/profile.html
(function(){
  const api = (url, opts) => {
    if (window.HostBerry?.apiRequest) return window.HostBerry.apiRequest(url, opts);
    const o = Object.assign({}, opts);
    if (o.body && typeof o.body === 'object' && !(o.body instanceof FormData)) o.body = JSON.stringify(o.body);
    return fetch(url, o);
  };

  function bindForm(formId, toBody){
    const form = document.getElementById(formId);
    if(!form) return;
    form.addEventListener('submit', async function(e){
      e.preventDefault();
      const fd = new FormData(this);
      const data = toBody(fd);
      const url = this.getAttribute('action') || form.dataset.action || window.location.pathname;
      try{
        const resp = await api(url, { method: this.getAttribute('method') || 'POST', body: data });
        if(resp?.ok){ HostBerry.showAlert('success', HostBerry.t('messages.changes_saved')); }
        else { HostBerry.showAlert('danger', HostBerry.t('errors.configuration_error')); }
      }catch(_e){ HostBerry.showAlert('danger', HostBerry.t('errors.network_error')); }
    });
  }

  // Perfil
  bindForm('profileForm', (fd)=>({
    email: fd.get('email'), first_name: fd.get('first_name'), last_name: fd.get('last_name'), timezone: fd.get('timezone')
  }));

  // Cambio contraseña
  const passwordForm = document.getElementById('passwordForm');
  if(passwordForm){
    passwordForm.addEventListener('submit', async function(e){
      e.preventDefault();
      const fd = new FormData(this);
      const newPassword = fd.get('new_password');
      const confirmPassword = fd.get('confirm_password');
      if(newPassword !== confirmPassword){ HostBerry.showAlert('danger', HostBerry.t('auth.password_mismatch')); return; }
      try{
        const resp = await api('/api/v1/auth/change-password', {
          method: 'POST',
          body: { current_password: fd.get('current_password'), new_password: newPassword }
        });
        if(resp?.ok){ HostBerry.showAlert('success', HostBerry.t('auth.password_changed')); this.reset(); }
        else { HostBerry.showAlert('danger', HostBerry.t('errors.operation_failed')); }
      }catch(_e){ HostBerry.showAlert('danger', HostBerry.t('errors.network_error')); }
    });
  }

  // Notificaciones
  bindForm('notificationForm', (fd)=>({
    email_notifications: fd.get('email_notifications') === 'on',
    system_alerts: fd.get('system_alerts') === 'on',
    security_alerts: fd.get('security_alerts') === 'on'
  }));

  // Privacidad
  bindForm('privacyForm', (fd)=>({
    show_activity: fd.get('show_activity') === 'on',
    data_collection: fd.get('data_collection') === 'on',
    analytics: fd.get('analytics') === 'on'
  }));
})();

