// JS para la página first-login con estética igual al login
(function(){
  // Cerrar dropdown de idioma al hacer clic fuera
  document.addEventListener('click', function(event) {
    const langDropdown = document.querySelector('.lang-dropdown');
    if (langDropdown && !langDropdown.contains(event.target)) {
      langDropdown.classList.remove('show');
    }
  });

  // Sistema de traducciones mejorado
  function t(key, defaultValue = '') {
    if (!key) return defaultValue || '';
    
    // 1. Primero intentar obtener del elemento i18n-data (atributos data-*)
    const i18nData = document.getElementById('i18n-data');
    if (i18nData) {
      const dataKey = key.replace(/\./g, '-');
      const value = i18nData.getAttribute(`data-${dataKey}`);
      if (value) {
        return value;
      }
    }
    
    // 2. Intentar obtener del JSON embebido (como en common.js)
    try {
      const i18nJson = document.getElementById('i18n-json');
      if (i18nJson) {
        const translations = JSON.parse(i18nJson.textContent || i18nJson.innerText || '{}');
        const keys = String(key).split('.');
        let current = translations;
        for (const k of keys) {
          if (current && Object.prototype.hasOwnProperty.call(current, k)) {
            current = current[k];
          } else {
            break;
          }
        }
        if (typeof current === 'string') {
          return current;
        }
      }
    } catch (e) {
      // Ignorar errores de parsing
    }
    
    // 3. Fallback al sistema anterior (window.i18nData)
    const keys = key.split('.');
    let current = window.i18nData || {};
    
    for (const k of keys) {
      if (current && typeof current === 'object' && k in current) {
        current = current[k];
      } else {
        return defaultValue || key;
      }
    }
    
    return (typeof current === 'string' ? current : null) || defaultValue || key;
  }

  // Función para mostrar alertas (mismo estilo que login.js)
  function showAlert(type, message) {
    // Añadir estilos CSS específicos si no existen
    if (!document.querySelector('#first-login-alert-styles')) {
      const style = document.createElement('style');
      style.id = 'first-login-alert-styles';
      style.textContent = `
        .first-login-alert {
          position: fixed !important;
          top: 20px !important;
          right: 20px !important;
          left: auto !important;
          bottom: auto !important;
          z-index: 9999 !important;
          min-width: 300px;
          max-width: 400px;
          margin: 0 !important;
          transform: none !important;
        }
      `;
      document.head.appendChild(style);
    }
    
    const alertDiv = document.createElement('div');
    alertDiv.className = `alert alert-${type} alert-dismissible fade show first-login-alert`;
    alertDiv.innerHTML = `
      ${message}
      <button type="button" class="btn-close" data-bs-dismiss="alert" aria-label="Close"></button>
    `;
    document.body.appendChild(alertDiv);
    setTimeout(() => {
      if (alertDiv.parentNode) {
        alertDiv.remove();
      }
    }, 5000);
  }

  // Función para mostrar notificación de éxito
  function showSuccess(message) {
    showAlert('success', message);
  }

  // Función para mostrar notificación de error
  function showError(message) {
    showAlert('danger', message);
  }

  // Función para mostrar notificación de información
  function showInfo(message) {
    showAlert('info', message);
  }
  
  // Función para mostrar notificación de advertencia
  function showWarning(message) {
    showAlert('warning', message);
  }
  
  // Función para mostrar notificaciones toast (compatibilidad)
  function showToast(title, message, type = 'info') {
    const alertType = type === 'success' ? 'success' : type === 'error' || type === 'danger' ? 'danger' : type === 'warning' ? 'warning' : 'info';
    showAlert(alertType, `${title}: ${message}`);
  }

  // Función para procesar errores de validación de Pydantic (con traducciones)
  function processValidationError(errorDetail) {
    if (Array.isArray(errorDetail)) {
      // Es un array de errores de validación de Pydantic
      const messages = errorDetail.map(error => {
        const field = error.loc && error.loc.length > 1 ? error.loc[1] : 'field';
        let message = error.msg || t('errors.validation_error', 'Error de validación');
        
        // Traducir nombres de campos
        const fieldNames = {
          'new_username': t('auth.username', 'Usuario'),
          'new_password': t('auth.password', 'Contraseña'),
          'confirm_password': t('auth.confirm_password', 'Confirmar contraseña')
        };
        
        // Traducir mensajes de error comunes de Pydantic
        const errorMessages = {
          'field required': t('errors.field_required', 'Este campo es requerido'),
          'string does not match expected pattern': t('errors.invalid_format', 'Formato inválido'),
          'string too short': t('errors.too_short', 'Demasiado corto'),
          'string too long': t('errors.too_long', 'Demasiado largo'),
          'value is not a valid string': t('errors.invalid_string', 'No es un texto válido'),
          'value is not a valid integer': t('errors.invalid_integer', 'No es un número válido'),
        };
        
        // Intentar traducir el mensaje de error
        const lowerMsg = message.toLowerCase();
        for (const [key, translation] of Object.entries(errorMessages)) {
          if (lowerMsg.includes(key)) {
            message = translation;
            break;
          }
        }
        
        // Si el mensaje contiene información sobre el campo, traducirlo
        const fieldName = fieldNames[field] || field;
        
        // Traducir mensajes específicos de validación
        if (message.includes('required')) {
          message = t('errors.field_required', 'Este campo es requerido');
        } else if (message.includes('too short') || message.includes('minimum')) {
          if (field === 'new_username') {
            message = t('errors.username_too_short', 'El nombre de usuario debe tener al menos 3 caracteres');
          } else if (field === 'new_password' || field === 'confirm_password') {
            message = t('errors.password_length', 'La contraseña debe tener al menos 8 caracteres');
          }
        } else if (message.includes('too long') || message.includes('maximum')) {
          if (field === 'new_username') {
            message = t('errors.username_too_long', 'El nombre de usuario no puede exceder 50 caracteres');
          }
        }
        
        return `${fieldName}: ${message}`;
      });
      
      return messages.join('\n');
    } else if (typeof errorDetail === 'string') {
      // Intentar traducir mensajes de error comunes
      const lowerMsg = errorDetail.toLowerCase();
      if (lowerMsg.includes('password') && lowerMsg.includes('match')) {
        return t('auth.passwords_dont_match', 'Las contraseñas no coinciden');
      }
      if (lowerMsg.includes('connection') || lowerMsg.includes('network')) {
        return t('errors.connection_error', 'Error de conexión');
      }
      if (lowerMsg.includes('validation')) {
        return t('errors.validation_error', 'Error de validación');
      }
      return errorDetail;
    } else if (typeof errorDetail === 'object') {
      if (errorDetail.message) {
        return processValidationError(errorDetail.message);
      }
      if (errorDetail.error) {
        return processValidationError(errorDetail.error);
      }
      return t('errors.validation_error', 'Error de validación');
    }
    
    return t('errors.validation_error', 'Error de validación');
  }

  function attachToggle(btnId, inputId, emojiId){
    const btn = document.getElementById(btnId);
    const input = document.getElementById(inputId);
    const emoji = document.getElementById(emojiId);
    if(!btn || !input || !emoji) return;
    
    btn.addEventListener('click', function(){
      const isPass = input.getAttribute('type') === 'password';
      input.setAttribute('type', isPass ? 'text' : 'password');
      emoji.textContent = isPass ? '🙈' : '👁️';
      const hideText = t('common.hide_password', 'Ocultar contraseña');
      const showText = t('common.show_password', 'Mostrar contraseña');
      emoji.setAttribute('title', isPass ? hideText : showText);
      btn.setAttribute('aria-label', isPass ? hideText : showText);
      btn.setAttribute('title', isPass ? hideText : showText);
    });
  }

  // Función para cambiar tema
  function toggleTheme() {
    const body = document.body;
    const themeToggle = document.getElementById('theme-toggle');
    const themeEmoji = document.getElementById('theme-emoji');
    
    if (body.classList.contains('dark-theme')) {
      body.classList.remove('dark-theme');
      body.classList.add('light-theme');
      themeToggle.classList.remove('dark');
      themeToggle.classList.add('light');
      themeEmoji.textContent = '🌙';
      localStorage.setItem('theme', 'light');
    } else {
      body.classList.remove('light-theme');
      body.classList.add('dark-theme');
      themeToggle.classList.remove('light');
      themeToggle.classList.add('dark');
      themeEmoji.textContent = '☀️';
      localStorage.setItem('theme', 'dark');
    }
  }

  // Función para aplicar tema guardado
  function applySavedTheme() {
    const savedTheme = localStorage.getItem('theme') || 'dark';
    const body = document.body;
    const themeToggle = document.getElementById('theme-toggle');
    const themeEmoji = document.getElementById('theme-emoji');
    
    if (savedTheme === 'light') {
      body.classList.remove('dark-theme');
      body.classList.add('light-theme');
      themeToggle.classList.remove('dark');
      themeToggle.classList.add('light');
      themeEmoji.textContent = '🌙';
    } else {
      body.classList.remove('light-theme');
      body.classList.add('dark-theme');
      themeToggle.classList.remove('light');
      themeToggle.classList.add('dark');
      themeEmoji.textContent = '☀️';
    }
  }

  document.addEventListener('DOMContentLoaded', function(){
    // Aplicar tema guardado
    applySavedTheme();
    
    // Configurar botones de mostrar/ocultar contraseña
    attachToggle('toggle-new-password', 'new_password', 'eye-emoji-new');
    attachToggle('toggle-confirm-password', 'confirm_password', 'eye-emoji-confirm');
    
    // Configurar cambio de tema
    const themeToggle = document.getElementById('theme-toggle');
    if (themeToggle) {
      themeToggle.addEventListener('click', toggleTheme);
    }
    
    const form = document.getElementById('firstLoginForm');
    
    if(!form) return;
    
      // Usar fetch directamente para tener control total sobre la respuesta
      const headers = {
        'Content-Type': 'application/json',
        'Accept-Language': currentLang
      };

      // Agregar token si existe (evitar Bearer null/undefined)
      if (token && token !== 'null' && token !== 'undefined') {
        headers['Authorization'] = `Bearer ${token}`;
      }

      const resp = await fetch('/api/v1/auth/first-login/change', {
        method: 'POST',
        headers: headers,
        credentials: 'include',
        body: JSON.stringify(payload)
      });

      let data = null;
      try {
        // Leer el texto de la respuesta primero
        const text = await resp.text();
        if (text && text.trim()) {
          try {
            data = JSON.parse(text);
          } catch (parseErr) {
            // Si no es JSON válido, podría ser HTML o texto plano
            console.warn('Response is not valid JSON:', text.substring(0, 100));
            data = null;
          }
        }
      } catch (_jsonErr) {
        console.error('Error reading response:', _jsonErr);
        data = null;
      }

      if(resp && resp.ok){
        // Extraer solo el mensaje de la respuesta
        let successMessage = t('auth.credentials_updated', 'Credenciales actualizadas. Vuelve a iniciar sesión.');

        if (data) {
          // Si data es un objeto con message, usar ese
          if (typeof data === 'object' && data !== null && data.message && typeof data.message === 'string') {
            successMessage = data.message;
          } 
          // Si data es un string, usarlo directamente
          else if (typeof data === 'string') {
            successMessage = data;
          }
          // Si data es un objeto pero no tiene message, verificar si es el objeto de traducciones
          else if (typeof data === 'object' && data !== null) {
            // Verificar si tiene las claves típicas de traducciones (adblock, auth, etc.)
            const hasTranslationKeys = data.adblock || data.auth || data.common || data.dashboard;
            if (hasTranslationKeys) {
              // Es el objeto de traducciones, no mostrar
              console.warn('Received translations object instead of success message, using default message');
            } else if (!data.message) {
              // No es traducciones pero tampoco tiene message, usar por defecto
              console.log('Response data (not shown to user):', Object.keys(data));
            }
          }
        }

        // Asegurar que el mensaje es un string y no un objeto
        if (typeof successMessage !== 'string') {
          successMessage = t('auth.credentials_updated', 'Credenciales actualizadas. Vuelve a iniciar sesión.');
        }

        showSuccess(successMessage);
        localStorage.removeItem('access_token');
        setTimeout(function(){ 
          window.location.href = `/login?lang=${encodeURIComponent(currentLang)}`;
        }, 1200);
      } else {
        // Procesar errores de validación de Pydantic (si hay JSON)
        const detail = data ? data.detail : null;
        if (detail) {
          showError(processValidationError(detail));
        } else {
          // Evitar mostrar por error un objeto de traducciones completo
          if (data && typeof data === 'object' && data !== null) {
            const hasTranslationKeys = data.adblock || data.auth || data.common || data.dashboard || data.errors;
            if (hasTranslationKeys) {
              data = null;
            }
          }

          // Caso especial: endpoint de first-login solo válido en primer login
          if (resp && resp.status === 403 && data && typeof data === 'object' && data !== null && typeof data.error === 'string') {
            showError(data.error);
            setTimeout(function(){
              window.location.href = `/login?lang=${encodeURIComponent(currentLang)}`;
            }, 1200);
            return;
          }

          // Caso: 401 sin token/cookie
          if (resp && resp.status === 401) {
            showError(t('auth.session_expired', 'Session expired'));
            setTimeout(function(){
              window.location.href = `/login?lang=${encodeURIComponent(currentLang)}`;
            }, 1200);
            return;
          }

          // Fallback: status + mensaje genérico
          const status = resp && typeof resp.status === 'number' ? resp.status : 0;
          const baseMsg = t('errors.general_error_message', 'Ha ocurrido un error inesperado');
          showError(status ? `${baseMsg} (HTTP ${status})` : baseMsg);
        }
      }
    }
    catch(_e){
      console.error('Error en first-login:', _e);
      const errorMsg = _e.message || t('errors.unknown_error', 'Error desconocido');
      showError(t('errors.connection_error', 'Error de conexión') + ': ' + errorMsg);
    }
  });
});
