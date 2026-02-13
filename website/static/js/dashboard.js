// Dashboard JavaScript
(function() {
    const apiRequestFn = window.HostBerry && window.HostBerry.apiRequest
        ? window.HostBerry.apiRequest
        : async function(url) {
            const token = localStorage.getItem('access_token');
            const headers = { 'Content-Type': 'application/json' };
            if (token) headers['Authorization'] = 'Bearer ' + token;
            return fetch(url, { headers: headers });
        };

    const formatUptime = (seconds) => {
        if (window.HostBerry && window.HostBerry.formatUptime) {
            return window.HostBerry.formatUptime(seconds);
        }
        if (!Number.isFinite(seconds) || seconds < 0) return '--';
        const d = Math.floor(seconds / 86400);
        const h = Math.floor((seconds % 86400) / 3600);
        const m = Math.floor((seconds % 3600) / 60);
        if (d > 0) return d + 'd ' + h + 'h ' + m + 'm';
        if (h > 0) return h + 'h ' + m + 'm';
        return m + 'm';
    };

    const setText = (id, value) => {
        const el = document.getElementById(id);
        if (el) el.textContent = value;
    };

    const setProgress = (id, percent) => {
        const bar = document.getElementById(id);
        if (bar && Number.isFinite(percent)) {
            const clamped = Math.min(100, Math.max(0, percent));
            bar.style.width = clamped + '%';
        }
    };

    const escapeHtml = (s) => {
        const str = String(s ?? '');
        return str.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;').replace(/'/g, '&#39;');
    };

    const updateTime = () => {
        const now = new Date();
        const timeStr = now.toLocaleTimeString();
        setText('dashboard-time', timeStr);
    };

    // Función de traducción local - carga traducciones una vez
    let translationsCache = null;
    const loadTranslations = () => {
        if (translationsCache) return translationsCache;
        try {
            const el = document.getElementById('i18n-json');
            if (el) {
                translationsCache = JSON.parse(el.textContent || el.innerText || '{}');
                return translationsCache;
            }
        } catch (e) {
            console.error('Error loading translations:', e);
        }
        return {};
    };

    const t = (key, defaultValue) => {
        // Primero intentar usar window.HostBerry.t si está disponible
        if (window.HostBerry && window.HostBerry.t) {
            const translated = window.HostBerry.t(key, defaultValue);
            if (translated && translated !== key && translated !== defaultValue) {
                return translated;
            }
        }
        
        // Fallback: cargar traducciones directamente
        const translations = loadTranslations();
        const parts = String(key).split('.');
        let cur = translations;
        for (const part of parts) {
            if (cur && Object.prototype.hasOwnProperty.call(cur, part)) {
                cur = cur[part];
            } else {
                return defaultValue || key;
            }
        }
        return typeof cur === 'string' ? cur : (defaultValue || key);
    };

    const updateHealth = (type, value, thresholds) => {
        const dot = document.getElementById('health-' + type);
        const text = document.getElementById('health-' + type + '-text');
        if (!dot || !text) return;

        let status = 'success';
        let statusText = t('dashboard.healthy', 'Healthy');
        
        if (value >= thresholds.critical) {
            status = 'danger';
            statusText = t('dashboard.critical', 'Critical');
        } else if (value >= thresholds.warning) {
            status = 'warning';
            statusText = t('dashboard.warning', 'Warning');
        }

        dot.className = 'health-dot health-dot-' + status;
        text.textContent = statusText;
    };

    const updateServiceHealth = (serviceName, isActive) => {
        const dot = document.getElementById('health-' + serviceName);
        const text = document.getElementById('health-' + serviceName + '-text');
        if (!dot || !text) return;

        const status = isActive ? 'success' : 'danger';
        const statusText = isActive 
            ? t('common.active', 'Active')
            : t('common.inactive', 'Inactive');

        dot.className = 'health-dot health-dot-' + status;
        text.textContent = statusText;
    };

    const updateServiceStatCard = (serviceName, isActive) => {
        const valueEl = document.getElementById('stat-' + serviceName);
        const barEl = document.getElementById('stat-' + serviceName + '-bar');
        const iconEl = document.getElementById('stat-' + serviceName + '-icon');
        
        if (valueEl) {
            valueEl.textContent = isActive ? t('common.active', 'Active') : t('common.inactive', 'Inactive');
            valueEl.className = 'stat-value' + (isActive ? ' text-success' : ' text-danger');
        }
        
        if (barEl) {
            barEl.style.width = isActive ? '100%' : '0%';
            barEl.className = 'stat-progress-bar' + (isActive ? ' bg-success' : ' bg-danger');
        }
        
        if (iconEl) {
            const iconMap = {
                'wifi': isActive ? 'bi-wifi-fill' : 'bi-wifi-off',
                'hostapd': 'bi-router',
                'dnsmasq': 'bi-server',
                'network': 'bi-hdd-network'
            };
            const iconClass = iconMap[serviceName] || 'bi-circle';
            iconEl.className = 'bi ' + iconClass + (isActive ? ' text-success' : ' text-danger');
        }
    };

    async function loadServiceStatus() {
        try {
            const servicesResp = await apiRequestFn('/api/v1/system/services');
            if (servicesResp && servicesResp.ok) {
                const servicesData = await servicesResp.json();
                const services = servicesData.services || {};
                
                // HostAPD
                if (services.hostapd) {
                    const hostapd = services.hostapd;
                    const isActive = hostapd.active === true || hostapd.status === 'active';
                    updateServiceStatCard('hostapd', isActive);
                } else {
                    updateServiceStatCard('hostapd', false);
                }
                
                // dnsmasq (usar datos ya obtenidos - adblock puede ser dnsmasq)
                if (services.adblock && services.adblock.type === 'dnsmasq') {
                    updateServiceStatCard('dnsmasq', services.adblock.active === true);
                } else if (services.dnsmasq) {
                    const dm = services.dnsmasq;
                    updateServiceStatCard('dnsmasq', dm.active === true || dm.status === 'active');
                } else {
                    updateServiceStatCard('dnsmasq', false);
                }
            } else {
                updateServiceStatCard('hostapd', false);
                updateServiceStatCard('dnsmasq', false);
            }
            
            // Cargar estado de WiFi
            try {
                const wifiResp = await apiRequestFn('/api/v1/wifi/status');
                if (wifiResp && wifiResp.ok) {
                    const wifiData = await wifiResp.json();
                    const wifiEnabled = wifiData.enabled === true || wifiData.wifi_enabled === true;
                    updateServiceStatCard('wifi', wifiEnabled);
                } else {
                    updateServiceStatCard('wifi', false);
                }
            } catch (e) {
                console.error('Error loading WiFi status:', e);
                updateServiceStatCard('wifi', false);
            }
            
            // Cargar estado de Red (verificar interfaces activas)
            try {
                const networkResp = await apiRequestFn('/api/v1/network/interfaces');
                if (networkResp && networkResp.ok) {
                    const networkData = await networkResp.json();
                    const interfaces = Array.isArray(networkData) ? networkData : (networkData.interfaces || []);
                    const hasActiveInterface = interfaces.some(iface => {
                        const status = (iface.status || iface.state || '').toLowerCase();
                        return status === 'up' || status === 'connected' || status === 'running';
                    });
                    updateServiceStatCard('network', hasActiveInterface);
                } else {
                    updateServiceStatCard('network', false);
                }
            } catch (e) {
                console.error('Error loading network status:', e);
                updateServiceStatCard('network', false);
            }
        } catch (error) {
            console.error('Error loading service status:', error);
        }
    }

    async function loadServices() {
        try {
            const resp = await apiRequestFn('/api/v1/system/services');
            if (!resp.ok) throw new Error('Services request failed');
            
            const data = await resp.json();
            const services = data.services || {};
            
            // Actualizar estado de servicios en System Health
            if (services.wireguard) {
                const wireguard = services.wireguard;
                const isActive = wireguard.active === true || wireguard.status === true;
                updateServiceHealth('wireguard', isActive);
            }
            
            if (services.openvpn) {
                const openvpn = services.openvpn;
                const isActive = openvpn.active === true || openvpn.status === 'active';
                updateServiceHealth('openvpn', isActive);
            }
            
            if (services.hostapd) {
                const hostapd = services.hostapd;
                const isActive = hostapd.active === true || hostapd.status === 'active';
                updateServiceHealth('hostapd', isActive);
            }
            
            if (services.adblock) {
                const adblock = services.adblock;
                const isActive = adblock.active === true || adblock.status === true;
                updateServiceHealth('adblock', isActive);
            }
        } catch (error) {
            console.error('Error loading services:', error);
        }
    }

    async function fetchDashboardData() {
        try {
            // Obtener stats e info en paralelo
            const [statsResp, infoResp] = await Promise.all([
                apiRequestFn('/api/v1/system/stats'),
                apiRequestFn('/api/v1/system/info')
            ]);
            
            if (!statsResp.ok) {
                console.error('Stats request failed:', statsResp.status, statsResp.statusText);
                throw new Error('Stats request failed: ' + statsResp.status);
            }
            
            if (!infoResp.ok) {
                console.error('Info request failed:', infoResp.status, infoResp.statusText);
            }
            
            const statsPayload = await statsResp.json();
            const infoPayload = infoResp.ok ? await infoResp.json() : {};
            
            // Manejar diferentes formatos de respuesta
            let stats = statsPayload;
            if (statsPayload.data) {
                stats = statsPayload.data;
            } else if (statsPayload.stats) {
                stats = statsPayload.stats;
            }
            
            let info = infoPayload;
            if (infoPayload.data) {
                info = infoPayload.data;
            } else if (infoPayload.info) {
                info = infoPayload.info;
            }

            // CPU - intentar diferentes nombres de campo
            const cpuUsage = stats.cpu_usage || stats.cpu_percent || stats.cpu || 0;
            setText('stat-cpu', cpuUsage.toFixed(1) + '%');
            setProgress('stat-cpu-bar', cpuUsage);
            updateHealth('cpu', cpuUsage, { warning: 70, critical: 90 });

            // Memory - intentar diferentes nombres de campo
            const memUsage = stats.memory_usage || stats.memory_percent || stats.memory || 0;
            setText('stat-memory', memUsage.toFixed(1) + '%');
            setProgress('stat-memory-bar', memUsage);
            updateHealth('memory', memUsage, { warning: 75, critical: 90 });

            // Disk - intentar diferentes nombres de campo
            const diskUsage = stats.disk_usage || stats.disk_percent || stats.disk || 0;
            setText('stat-disk', diskUsage.toFixed(1) + '%');
            setProgress('stat-disk-bar', diskUsage);
            updateHealth('disk', diskUsage, { warning: 80, critical: 95 });

            // Uptime
            const uptimeSeconds = info.uptime_seconds || stats.uptime || stats.uptime_seconds || 0;
            const uptimeFormatted = formatUptime(uptimeSeconds);
            setText('stat-uptime', uptimeFormatted);

            // Cargar estado de servicios para las nuevas tarjetas
            loadServiceStatus();

            // System Info - combinar datos de stats e info
            setText('info-hostname', info.hostname || stats.hostname || stats.host_name || '--');
            setText('info-os', info.os_version || stats.os_version || stats.os || '--');
            setText('info-kernel', info.kernel_version || stats.kernel_version || stats.kernel || '--');
            setText('info-arch', info.architecture || stats.architecture || stats.arch || '--');
            setText('info-uptime', formatUptime(info.uptime_seconds || stats.uptime || stats.uptime_seconds || 0));
            setText('info-cores', stats.cpu_cores || stats.cores || stats.cpu_count || info.cpu_cores || '--');

        } catch (error) {
            console.error('Error fetching dashboard data:', error);
            // Mostrar mensaje de error al usuario
            if (window.HostBerry && window.HostBerry.showAlert) {
                window.HostBerry.showAlert('warning', 'Unable to load dashboard data. Please refresh the page.');
            }
        }
    }

    async function loadActivity() {
        const container = document.getElementById('activity-list');
        if (!container) return;
        
        try {
            const resp = await apiRequestFn('/api/v1/system/activity?limit=10');
            
            if (!resp.ok) {
                console.error('Activity request failed:', resp.status, resp.statusText);
                throw new Error('Activity request failed: ' + resp.status);
            }
            
            const activities = await resp.json();
            
            // El endpoint devuelve un array directamente, no envuelto
            const activitiesList = Array.isArray(activities) ? activities : (activities.activities || activities.data || []);

            if (!activitiesList.length) {
                const noActivityText = (window.HostBerry && window.HostBerry.t) ? window.HostBerry.t('dashboard.no_activity', 'No recent activity') : 'No recent activity';
                container.innerHTML = '<div class="activity-item"><div class="activity-content"><div class="activity-text">' + noActivityText + '</div></div></div>';
                return;
            }

            container.innerHTML = '';
            activitiesList.forEach(function(activity) {
                const item = document.createElement('div');
                item.className = 'activity-item';
                
                // Usar level en lugar de type
                const level = (activity.level || '').toLowerCase();
                const icon = level === 'error' ? 'bi-exclamation-triangle' : 
                           level === 'warning' ? 'bi-exclamation-circle' : 
                           level === 'info' ? 'bi-info-circle' : 'bi-check-circle';
                
                const time = activity.timestamp ? new Date(activity.timestamp).toLocaleString() : '';
                const message = escapeHtml(activity.message || activity.description || activity.content || '');
                const source = activity.source ? ' [' + escapeHtml(activity.source) + ']' : '';
                const timeHtml = time ? '<div class="activity-time">' + escapeHtml(time) + '</div>' : '';
                
                item.innerHTML = '<div class="activity-icon"><i class="bi ' + icon + '"></i></div><div class="activity-content"><div class="activity-text">' + message + source + '</div>' + timeHtml + '</div>';
                container.appendChild(item);
            });
        } catch (error) {
            console.error('Error loading activity:', error);
            const errorText = (window.HostBerry && window.HostBerry.t) ? window.HostBerry.t('errors.unknown_error', 'Error loading activity') : 'Error loading activity';
            if (container) {
                container.innerHTML = '<div class="activity-item"><div class="activity-content"><div class="activity-text text-danger">' + errorText + '</div></div></div>';
            }
        }
    }

    // Esperar a que las traducciones estén cargadas
    function waitForTranslations(callback, maxAttempts = 50) {
        const el = document.getElementById('i18n-json');
        if (el || (window.HostBerry && window.HostBerry.t)) {
            callback();
        } else if (maxAttempts > 0) {
            setTimeout(() => waitForTranslations(callback, maxAttempts - 1), 100);
        } else {
            // Continuar de todos modos después de 5 segundos
            callback();
        }
    }

    document.addEventListener('DOMContentLoaded', function() {
        waitForTranslations(function() {
            updateTime();
            setInterval(updateTime, 1000);
            
            fetchDashboardData();
            loadActivity();
            loadServices();
            loadServiceStatus();
            
            const refreshSystemInfo = document.getElementById('refresh-system-info');
            if (refreshSystemInfo) {
                refreshSystemInfo.addEventListener('click', fetchDashboardData);
            }
            
            const refreshActivity = document.getElementById('refresh-activity');
            if (refreshActivity) {
                refreshActivity.addEventListener('click', loadActivity);
            }
            
            setInterval(fetchDashboardData, 30000);
            setInterval(loadActivity, 60000);
            setInterval(loadServices, 30000);
            setInterval(loadServiceStatus, 30000);
        });
    });
})();
