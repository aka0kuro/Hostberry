/**
 * HostBerry FastAPI - JavaScript Principal
 * Funciones comunes para toda la aplicación
 */

// Variables globales
let currentLanguage = 'es';
let translations = {};

// Función de traducción principal
function t(key, defaultValue = '') {
    const keys = key.split('.');
    let value = window.translations;
    
    for (const k of keys) {
        if (value && typeof value === 'object' && k in value) {
            value = value[k];
        } else {
            return defaultValue || key;
        }
    }
    
    return typeof value === 'string' ? value : (defaultValue || key);
}

// Función para mostrar alertas
function showAlert(type, message, duration = 5000) {
    const alertDiv = document.createElement('div');
    alertDiv.className = `alert alert-${type} alert-dismissible fade show`;
    alertDiv.innerHTML = `
        ${message}
        <button type="button" class="btn-close" data-bs-dismiss="alert"></button>
    `;
    
    const container = document.querySelector('.container-fluid') || document.querySelector('.container');
    if (container) {
        container.insertBefore(alertDiv, container.firstChild);
        
        setTimeout(() => {
            if (alertDiv.parentNode) {
                alertDiv.remove();
            }
        }, duration);
    }
}

// Función para mostrar notificaciones toast
function showToast(title, message, type = 'info') {
    const toastContainer = document.querySelector('.toast-container');
    if (!toastContainer) {
        const container = document.createElement('div');
        container.className = 'toast-container position-fixed top-0 end-0 p-3';
        document.body.appendChild(container);
    }
    
    const toastId = 'toast-' + Date.now();
    const toastHtml = `
        <div id="${toastId}" class="toast" role="alert" aria-live="assertive" aria-atomic="true">
            <div class="toast-header">
                <strong class="me-auto">${title}</strong>
                <button type="button" class="btn-close" data-bs-dismiss="toast"></button>
            </div>
            <div class="toast-body">${message}</div>
        </div>
    `;
    
    toastContainer.insertAdjacentHTML('beforeend', toastHtml);
    const toastElement = document.getElementById(toastId);
    const toast = new bootstrap.Toast(toastElement);
    toast.show();
    
    // Auto-remove after 5 seconds
    setTimeout(() => {
        if (toastElement.parentNode) {
            toastElement.remove();
        }
    }, 5000);
}

// Función para hacer peticiones HTTP
async function apiRequest(url, options = {}) {
    const token = localStorage.getItem('access_token');
    const headers = {
        'Content-Type': 'application/json'
    };
    if (token && token !== 'null' && token !== 'undefined') {
        headers['Authorization'] = `Bearer ${token}`;
    }
    const defaultOptions = {
        credentials: 'include',
        headers
    };
    
    const finalOptions = { ...defaultOptions, ...options };
    
    try {
        const response = await fetch(url, finalOptions);
        
        if (!response.ok) {
            throw new Error(`HTTP error! status: ${response.status}`);
        }
        
        return await response.json();
    } catch (error) {
        console.error('API Request Error:', error);
        throw error;
    }
}

// Función para cargar datos con loading
async function loadDataWithLoading(url, loadingElement, successCallback, errorCallback) {
    if (loadingElement) {
        loadingElement.innerHTML = '<div class="loading"></div>';
    }
    
    try {
        const data = await apiRequest(url);
        if (successCallback) {
            successCallback(data);
        }
    } catch (error) {
        if (errorCallback) {
            errorCallback(error);
        } else {
            showAlert('danger', t('errors.network_error'));
        }
    } finally {
        if (loadingElement) {
            loadingElement.innerHTML = '';
        }
    }
}

// Función para validar formularios
function validateForm(formElement) {
    const inputs = formElement.querySelectorAll('input[required], select[required], textarea[required]');
    let isValid = true;
    
    inputs.forEach(input => {
        if (!input.value.trim()) {
            input.classList.add('is-invalid');
            isValid = false;
        } else {
            input.classList.remove('is-invalid');
        }
    });
    
    return isValid;
}

// Función para limpiar formularios
function clearForm(formElement) {
    formElement.reset();
    formElement.querySelectorAll('.is-invalid').forEach(input => {
        input.classList.remove('is-invalid');
    });
}

// Función para cambiar idioma
function changeLanguage(language) {
    currentLanguage = language;
    localStorage.setItem('language', language);
    
    // Recargar la página para aplicar el nuevo idioma
    const currentUrl = new URL(window.location);
    currentUrl.searchParams.set('lang', language);
    window.location.href = currentUrl.toString();
}

// Función para obtener color de estado
function getStatusColor(status) {
    switch (status.toLowerCase()) {
        case 'online':
        case 'running':
        case 'active':
        case 'connected':
        case 'healthy':
            return 'success';
        case 'offline':
        case 'stopped':
        case 'inactive':
        case 'disconnected':
        case 'unhealthy':
            return 'danger';
        case 'warning':
        case 'degraded':
            return 'warning';
        default:
            return 'secondary';
    }
}

// Función para formatear bytes
function formatBytes(bytes, decimals = 2) {
    if (bytes === 0) return '0 Bytes';
    
    const k = 1024;
    const dm = decimals < 0 ? 0 : decimals;
    const sizes = ['Bytes', 'KB', 'MB', 'GB', 'TB', 'PB', 'EB', 'ZB', 'YB'];
    
    const i = Math.floor(Math.log(bytes) / Math.log(k));
    
    return parseFloat((bytes / Math.pow(k, i)).toFixed(dm)) + ' ' + sizes[i];
}

// Función para formatear tiempo
function formatUptime(seconds) {
    const days = Math.floor(seconds / 86400);
    const hours = Math.floor((seconds % 86400) / 3600);
    const minutes = Math.floor((seconds % 3600) / 60);
    
    if (days > 0) {
        return `${days}d ${hours}h ${minutes}m`;
    } else if (hours > 0) {
        return `${hours}h ${minutes}m`;
    } else {
        return `${minutes}m`;
    }
}

// Función para formatear fecha
function formatDate(dateString) {
    const date = new Date(dateString);
    return date.toLocaleDateString(currentLanguage === 'es' ? 'es-ES' : 'en-US', {
        year: 'numeric',
        month: 'short',
        day: 'numeric',
        hour: '2-digit',
        minute: '2-digit'
    });
}

// Función para actualizar estadísticas en tiempo real
function updateStats() {
    const statsElements = document.querySelectorAll('[data-stat]');
    
    statsElements.forEach(element => {
        const statType = element.dataset.stat;
        const url = `/api/v1/stats/${statType}`;
        
        loadDataWithLoading(url, null, (data) => {
            element.textContent = data.value;
            element.className = `text-${getStatusColor(data.status)}`;
        });
    });
}

// Función para inicializar tooltips
function initTooltips() {
    const tooltipTriggerList = [].slice.call(document.querySelectorAll('[data-bs-toggle="tooltip"]'));
    tooltipTriggerList.map(function (tooltipTriggerEl) {
        return new bootstrap.Tooltip(tooltipTriggerEl);
    });
}

// Función para inicializar popovers
function initPopovers() {
    const popoverTriggerList = [].slice.call(document.querySelectorAll('[data-bs-toggle="popover"]'));
    popoverTriggerList.map(function (popoverTriggerEl) {
        return new bootstrap.Popover(popoverTriggerEl);
    });
}

// Función para inicializar modales
function initModals() {
    const modalTriggerList = [].slice.call(document.querySelectorAll('[data-bs-toggle="modal"]'));
    modalTriggerList.map(function (modalTriggerEl) {
        return new bootstrap.Modal(modalTriggerEl);
    });
}

// Función para manejar errores de red
function handleNetworkError(error) {
    console.error('Network Error:', error);
    showAlert('danger', t('errors.network_error'));
}

// Función para manejar errores de autenticación
function handleAuthError() {
    showAlert('warning', t('auth.session_expired'));
    setTimeout(() => {
        window.location.href = '/login';
    }, 2000);
}

// Función para verificar autenticación
function checkAuth() {
    const token = localStorage.getItem('access_token');
    if (!token) {
        window.location.href = '/login';
        return false;
    }
    return true;
}

// Función para cerrar sesión
function logout() {
    localStorage.removeItem('access_token');
    localStorage.removeItem('user_info');
    window.location.href = '/login';
}

// Función para inicializar la aplicación
function initApp() {
    // Cargar idioma guardado
    const savedLanguage = localStorage.getItem('language');
    if (savedLanguage) {
        currentLanguage = savedLanguage;
    }
    
    // Inicializar componentes de Bootstrap
    initTooltips();
    initPopovers();
    initModals();
    
    // Configurar interceptores de errores
    window.addEventListener('error', (event) => {
        console.error('Global Error:', event.error);
        showAlert('danger', t('errors.general_error'));
    });
    
    // Configurar auto-refresh para estadísticas
    if (document.querySelector('[data-stat]')) {
        setInterval(updateStats, 30000); // Actualizar cada 30 segundos
    }
    
    // Configurar eventos de formularios
    document.querySelectorAll('form').forEach(form => {
        form.addEventListener('submit', (e) => {
            if (!validateForm(form)) {
                e.preventDefault();
                showAlert('warning', t('errors.validation_error'));
            }
        });
    });
    
    // Configurar eventos de botones de logout
    document.querySelectorAll('[data-action="logout"]').forEach(button => {
        button.addEventListener('click', logout);
    });
    
    // Configurar eventos de cambio de idioma
    document.querySelectorAll('[data-action="change-language"]').forEach(button => {
        button.addEventListener('click', (e) => {
            e.preventDefault();
            const language = button.dataset.language;
            changeLanguage(language);
        });
    });
}

// Función para cargar traducciones
async function loadTranslations(language) {
    try {
        const response = await fetch(`/api/v1/translations/${language}`);
        if (response.ok) {
            translations = await response.json();
            window.translations = translations;
        }
    } catch (error) {
        console.error('Error loading translations:', error);
    }
}

// Inicializar cuando el DOM esté listo
document.addEventListener('DOMContentLoaded', async function() {
    // Cargar traducciones
    await loadTranslations(currentLanguage);
    
    // Inicializar aplicación
    initApp();
    
    // Mostrar mensaje de bienvenida si es la primera visita
    if (!localStorage.getItem('visited')) {
        showToast(
            t('common.welcome'),
            t('common.welcome_message'),
            'success'
        );
        localStorage.setItem('visited', 'true');
    }
});

// Exportar funciones para uso global
window.HostBerry = {
    t,
    showAlert,
    showToast,
    apiRequest,
    loadDataWithLoading,
    validateForm,
    clearForm,
    changeLanguage,
    getStatusColor,
    formatBytes,
    formatUptime,
    formatDate,
    updateStats,
    checkAuth,
    logout,
    initApp
}; 