// System page JS (sin inline scripts)
(function () {
  'use strict';

  const HB = window.HostBerry;
  if (!HB) {
    console.warn('HostBerry no está disponible; system.js no se inicializa.');
    return;
  }

  const t = (key, def) => (HB.t ? HB.t(key, def) : def);
  const api = (url, opts) => {
    if (HB.apiRequest) return HB.apiRequest(url, opts);
    const o = Object.assign({}, opts);
    const token = localStorage.getItem('access_token');
    const headers = new Headers(o.headers || {});
    if (token && !headers.has('Authorization')) headers.set('Authorization', 'Bearer ' + token);
    if (!headers.has('Content-Type')) headers.set('Content-Type', 'application/json');
    o.headers = headers;
    if (o.body && typeof o.body === 'object' && !(o.body instanceof FormData)) o.body = JSON.stringify(o.body);
    return fetch(url, o);
  };
  const toast = (type, msg) => (HB.showAlert ? HB.showAlert(type, msg) : alert(msg));

  const setText = (id, value) => {
    const el = document.getElementById(id);
    if (el) el.textContent = value;
  };

  const setProgress = (id, percent) => {
    const bar = document.getElementById(id);
    if (bar && Number.isFinite(percent)) {
      const clamped = Math.min(100, Math.max(0, percent));
      bar.style.width = `${clamped}%`;
    }
  };

  const setStatusBadge = (id, status) => {
    const el = document.getElementById(id);
    if (!el) return;
    const badge = el.querySelector('.badge') || el;
    const statusLower = String(status || '').toLowerCase();
    let badgeClass = 'bg-secondary';
    let statusText = '--';

    if (statusLower === 'healthy' || statusLower === 'normal') {
      badgeClass = 'bg-success';
      statusText = t('system.healthy', 'Healthy');
    } else if (statusLower === 'warning') {
      badgeClass = 'bg-warning text-dark';
      statusText = t('system.warning', 'Warning');
    } else if (statusLower === 'critical') {
      badgeClass = 'bg-danger';
      statusText = t('system.critical', 'Critical');
    }

    badge.className = 'badge ' + badgeClass;
    badge.textContent = statusText;
  };

  const formatUptime = (s) => (HB.formatUptime ? HB.formatUptime(s) : (Number.isFinite(s) && s >= 0 ? `${Math.floor(s / 86400)}d ${Math.floor((s % 86400) / 3600)}h ${Math.floor((s % 3600) / 60)}m` : '--'));

  function calculateHealthStatus(value, thresholds) {
    if (value >= thresholds.critical) return 'critical';
    if (value >= thresholds.warning) return 'warning';
    return 'healthy';
  }

  async function fetchSystemStats() {
    try {
      const [statsResp, infoResp] = await Promise.all([api('/api/v1/system/stats'), api('/api/v1/system/info')]);

      if (!statsResp || !statsResp.ok) throw new Error('Stats request failed');
      const statsPayload = await statsResp.json();
      const infoPayload = infoResp && infoResp.ok ? await infoResp.json() : {};

      // Manejar diferentes formatos de respuesta
      const stats = statsPayload?.data || statsPayload?.stats || statsPayload || {};
      const info = infoPayload?.data || infoPayload?.info || infoPayload || {};

      // CPU
      const cpuUsage = Number(stats.cpu_usage ?? stats.cpu_percent ?? stats.cpu ?? 0) || 0;
      setText('system-cpu-percent', cpuUsage.toFixed(1) + '%');
      setProgress('system-cpu-progress', cpuUsage);
      setStatusBadge('system-cpu-status', calculateHealthStatus(cpuUsage, { warning: 70, critical: 90 }));

      // Memory
      const memUsage = Number(stats.memory_usage ?? stats.memory_percent ?? stats.memory ?? 0) || 0;
      setText('system-memory-percent', memUsage.toFixed(1) + '%');
      setProgress('system-memory-progress', memUsage);
      setStatusBadge('system-memory-status', calculateHealthStatus(memUsage, { warning: 75, critical: 90 }));

      // Disk
      const diskUsage = Number(stats.disk_usage ?? stats.disk_percent ?? stats.disk ?? 0) || 0;
      setText('system-disk-percent', diskUsage.toFixed(1) + '%');
      setProgress('system-disk-progress', diskUsage);
      setStatusBadge('system-disk-status', calculateHealthStatus(diskUsage, { warning: 80, critical: 95 }));

      // Temperature
      const temp = Number(stats.cpu_temperature ?? 0) || 0;
      setText('system-temperature', temp > 0 ? temp.toFixed(1) + '°C' : '--°C');
      if (temp > 0) {
        setProgress('system-temp-progress', (temp / 100) * 100);
        setStatusBadge('system-temp-status', calculateHealthStatus(temp, { warning: 60, critical: 80 }));
      }

      // System Info
      setText('info-hostname', info.hostname || stats.hostname || '--');
      setText('info-os-version', info.os_version || stats.os_version || '--');
      setText('info-kernel', info.kernel_version || stats.kernel_version || stats.kernel || '--');
      setText('info-architecture', info.architecture || stats.architecture || stats.arch || '--');
      setText('info-uptime', formatUptime(Number(info.uptime_seconds ?? stats.uptime ?? stats.uptime_seconds ?? 0) || 0));
      setText('info-cpu-cores', stats.cpu_cores || stats.cores || stats.cpu_count || info.cpu_cores || '--');

      const now = new Date();
      const timeStr = typeof HB.formatTime === 'function' ? HB.formatTime(now) : now.toLocaleTimeString();
      setText('system-last-update', timeStr);
    } catch (error) {
      console.error('Error fetching system stats:', error);
      toast('danger', t('errors.system_stats_error', 'Unable to fetch system statistics'));
    }
  }

  async function loadServices() {
    const container = document.getElementById('services-list');
    if (!container) return;

    try {
      const resp = await api('/api/v1/system/services');
      if (!resp || !resp.ok) throw new Error('Services request failed');
      const payload = await resp.json().catch(() => ({}));
      const services = payload.services || {};

      if (!Object.keys(services).length) {
        container.innerHTML = `
          <div class="text-center py-4 text-white-50">
            <i class="bi bi-journal-x"></i>
            <p class="mb-0 mt-2">${t('system.no_services', 'No services found')}</p>
          </div>`;
        return;
      }

      const serviceIcons = {
        hostberry: 'bi-gear-fill hostberry-icon',
        nginx: 'bi-shield-check nginx-icon',
        fail2ban: 'bi-shield-fill fail2ban-icon',
        ufw: 'bi-shield-lock ufw-icon',
        ssh: 'bi-terminal ssh-icon',
        hostapd: 'bi-wifi wifi-icon',
        openvpn: 'bi-shield-check openvpn-icon',
        'wg-quick': 'bi-shield-lock wireguard-icon',
        wireguard: 'bi-shield-lock wireguard-icon',
        dnsmasq: 'bi-diagram-3 dns-icon',
        adblock: 'bi-shield-x adblock-icon',
      };

      container.innerHTML = '';
      Object.keys(services).forEach((serviceName) => {
        const service = services[serviceName];
        const status = service.status || service;
        const statusLower = (typeof status === 'string' ? status : 'unknown').toLowerCase();
        const isRunning = statusLower === 'running' || statusLower === 'active';
        const safeServiceName = String(serviceName || '');

        const statusText = isRunning ? t('system.running', 'Running') : t('system.stopped', 'Stopped');
        const statusClass = isRunning ? 'bg-success' : 'bg-danger';

        const normalizedName = serviceName.toLowerCase().replace(/[_-]/g, '');
        const iconClass = serviceIcons[normalizedName] || serviceIcons[serviceName.toLowerCase()] || 'bi-gear-fill default-icon';

        const item = document.createElement('div');
        item.className = 'service-item';

        const serviceInfo = document.createElement('div');
        serviceInfo.className = 'service-info';

        const icon = document.createElement('i');
        icon.className = `bi ${iconClass} service-icon`;

        const nameEl = document.createElement('span');
        nameEl.className = 'service-name';
        nameEl.textContent = safeServiceName;

        serviceInfo.appendChild(icon);
        serviceInfo.appendChild(nameEl);

        const serviceControls = document.createElement('div');
        serviceControls.className = 'service-controls';

        const statusBadge = document.createElement('span');
        statusBadge.className = `badge ${statusClass}`;
        statusBadge.textContent = statusText;

        const actionBtn = document.createElement('button');
        actionBtn.className = `btn btn-sm ${isRunning ? 'btn-outline-danger' : 'btn-outline-success'}`;
        actionBtn.title = isRunning ? t('system.stop_service', 'Stop service') : t('system.start_service', 'Start service');
        actionBtn.innerHTML = `<i class="bi ${isRunning ? 'bi-stop' : 'bi-play'}"></i>`;
        actionBtn.addEventListener('click', () => toggleService(safeServiceName, isRunning));

        serviceControls.appendChild(statusBadge);
        serviceControls.appendChild(actionBtn);

        item.appendChild(serviceInfo);
        item.appendChild(serviceControls);
        container.appendChild(item);
      });
    } catch (error) {
      console.error('Services load failed', error);
      container.innerHTML = `
        <div class="text-center py-4 text-danger">
          <i class="bi bi-exclamation-triangle"></i>
          <p class="mb-0 mt-2">${t('system.services_error', 'Unable to load services')}</p>
        </div>`;
    }
  }

  async function toggleService(serviceName, isRunning) {
    try {
      const actionText = isRunning ? t('system.stop_service', 'Stop service') : t('system.start_service', 'Start service');
      const confirmMsg = (t('system.service_confirm', 'Are you sure you want to {action} {service}?') || '')
        .replace('{action}', actionText)
        .replace('{service}', serviceName);
      if (!confirm(confirmMsg)) return;

      // No hay endpoint estable para start/stop aún: dejamos feedback al usuario.
      toast('info', t('system.service_action_pending', 'Service action pending...'));
      await loadServices();
    } catch (error) {
      console.error('Service toggle failed', error);
      toast('danger', t('system.service_toggle_error', 'Unable to toggle service'));
    }
  }

  function getLevelColor(level) {
    switch (level) {
      case 'ERROR':
        return 'danger';
      case 'WARNING':
        return 'warning text-dark';
      case 'INFO':
        return 'info';
      default:
        return 'secondary';
    }
  }

  async function loadLogs() {
    const container = document.getElementById('system-logs');
    const level = document.getElementById('logLevel')?.value || 'all';
    if (!container) return;

    try {
      const params = new URLSearchParams({ limit: '50' });
      if (level !== 'all') params.set('level', level);

      const resp = await api(`/api/v1/system/logs?${params.toString()}`);
      if (!resp || !resp.ok) throw new Error('Logs request failed');

      const payload = await resp.json().catch(() => ({}));
      const logs = Array.isArray(payload.logs) ? payload.logs : Array.isArray(payload) ? payload : [];

      if (!logs.length) {
        container.innerHTML = `
          <div class="text-center py-4 text-white-50">
            <i class="bi bi-journal-x"></i>
            <p class="mb-0 mt-2">${t('system.no_logs', 'No logs found')}</p>
          </div>`;
        return;
      }

      container.innerHTML = '';
      logs.forEach((log) => {
        const item = document.createElement('div');
        item.className = 'log-item';

        const levelLabel = String(log.level || log.log_level || 'INFO').toUpperCase();
        const ts = log.timestamp ? new Date(log.timestamp) : log.created_at ? new Date(log.created_at) : null;
        const timeStr = ts ? ts.toLocaleTimeString(undefined, { hour: '2-digit', minute: '2-digit', second: '2-digit' }) : '--';
        const dateStr = ts ? ts.toLocaleDateString() : '--';
        const message = String(log.message || log.log_message || log.content || '');
        const safeLevelLabel = (HB.escapeHtml ? HB.escapeHtml(levelLabel) : levelLabel.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;'));
        const escapedMessage = (HB.escapeHtml ? HB.escapeHtml(message) : message.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;'));

        item.innerHTML = `
          <div class="d-flex align-items-center gap-2 flex-wrap">
            <span class="text-white-50 small" style="min-width: 80px;">${timeStr}</span>
            <span class="text-white-50 small" style="min-width: 90px;">${dateStr}</span>
            <span class="badge bg-${getLevelColor(levelLabel)} log-level">${safeLevelLabel}</span>
            <span class="text-white flex-grow-1">${escapedMessage}</span>
          </div>`;

        container.appendChild(item);
      });
    } catch (error) {
      console.error('Logs load failed', error);
      container.innerHTML = `
        <div class="text-center py-4 text-danger">
          <i class="bi bi-exclamation-triangle"></i>
          <p class="mb-0 mt-2">${t('system.logs_error', 'Unable to load logs')}</p>
        </div>`;
    }
  }

  async function createBackup() {
    try {
      if (!confirm(t('system.backup_confirm', 'Are you sure you want to create a system backup?'))) return;
      toast('info', t('system.backup_creating', 'Creating backup...'));
      const resp = await api('/api/v1/system/backup', { method: 'POST' });
      const payload = await resp?.json?.().catch(() => ({}));
      if (!resp || !resp.ok || payload?.success === false) {
        toast('warning', payload?.message || t('system.backup_error', 'Unable to create backup'));
        return;
      }
      toast('success', payload?.message || t('system.backup_success', 'Backup created successfully'));
    } catch (error) {
      console.error('Backup failed', error);
      toast('danger', t('system.backup_error', 'Unable to create backup'));
    }
  }

  async function restartSystem() {
    try {
      if (!confirm(t('system.restart_confirm', 'Are you sure you want to restart the system? This will disconnect all users.'))) return;
      toast('warning', t('system.restarting', 'Restarting system...'));
      const resp = await api('/api/v1/system/restart', { method: 'POST' });
      const payload = await resp?.json?.().catch(() => ({}));
      if (!resp || !resp.ok || payload?.success === false) {
        toast('danger', payload?.error || t('system.restart_error', 'Unable to restart system'));
        return;
      }
      toast('info', payload?.message || t('system.restart_pending', 'Restart command sent'));
      setTimeout(() => window.location.reload(), 5000);
    } catch (error) {
      console.error('Restart failed', error);
      toast('danger', t('system.restart_error', 'Unable to restart system'));
    }
  }

  async function shutdownSystem() {
    try {
      if (!confirm(t('system.shutdown_confirm', 'Are you sure you want to shutdown the system? This will disconnect all users.'))) return;
      const doubleConfirm = t('system.shutdown_double_confirm', 'Type SHUTDOWN to confirm');
      if (prompt(doubleConfirm) !== 'SHUTDOWN') return;
      toast('warning', t('system.shutting_down', 'Shutting down system...'));
      const resp = await api('/api/v1/system/shutdown', { method: 'POST' });
      const payload = await resp?.json?.().catch(() => ({}));
      if (!resp || !resp.ok || payload?.success === false) {
        toast('danger', payload?.error || t('system.shutdown_error', 'Unable to shutdown system'));
        return;
      }
      toast('info', payload?.message || t('system.shutdown_pending', 'Shutdown command sent'));
    } catch (error) {
      console.error('Shutdown failed', error);
      toast('danger', t('system.shutdown_error', 'Unable to shutdown system'));
    }
  }

  async function updateSystem() {
    try {
      if (!confirm(t('update.system_confirm', 'Update system?'))) return;
      toast('info', t('update.updating_system', 'Updating system...'));
      const resp = await api('/api/v1/system/updates/execute', { method: 'POST' });
      if (resp && (resp.status === 404 || resp.status === 405 || resp.status === 501)) {
        toast('warning', t('errors.not_implemented', 'Not implemented'));
        return;
      }
      const payload = await resp?.json?.().catch(() => ({}));
      if (!resp || !resp.ok || payload?.success === false) {
        toast('danger', payload?.detail || payload?.error || t('update.system_failed', 'Error updating system'));
        return;
      }
      toast('success', payload?.message || t('update.system_success', 'System updated'));
    } catch (error) {
      console.error('System update failed', error);
      toast('danger', t('update.system_failed', 'Error updating system'));
    }
  }

  async function updateProject() {
    try {
      if (!confirm(t('update.project_confirm', 'Update project?'))) return;
      toast('info', t('update.updating_project', 'Updating project...'));
      const resp = await api('/api/v1/system/updates/project', { method: 'POST' });
      if (resp && (resp.status === 404 || resp.status === 405 || resp.status === 501)) {
        toast('warning', t('errors.not_implemented', 'Not implemented'));
        return;
      }
      const payload = await resp?.json?.().catch(() => ({}));
      if (!resp || !resp.ok || payload?.success === false) {
        toast('danger', payload?.detail || payload?.error || t('update.project_failed', 'Error updating project'));
        return;
      }
      toast('success', payload?.message || t('update.project_success', 'Project updated'));
    } catch (error) {
      console.error('Project update failed', error);
      toast('danger', t('update.project_failed', 'Error updating project'));
    }
  }

  document.addEventListener('DOMContentLoaded', () => {
    fetchSystemStats();
    loadServices();
    loadLogs();

    document.getElementById('refreshSystemInfo')?.addEventListener('click', fetchSystemStats);
    document.getElementById('refreshServices')?.addEventListener('click', loadServices);
    document.getElementById('refreshLogs')?.addEventListener('click', loadLogs);
    document.getElementById('logLevel')?.addEventListener('change', loadLogs);

    document.getElementById('action-backup')?.addEventListener('click', createBackup);
    document.getElementById('action-update-system')?.addEventListener('click', updateSystem);
    document.getElementById('action-update-project')?.addEventListener('click', updateProject);
    document.getElementById('action-restart')?.addEventListener('click', restartSystem);
    document.getElementById('action-shutdown')?.addEventListener('click', shutdownSystem);

    setInterval(fetchSystemStats, 30000);
    setInterval(loadServices, 60000);
    setInterval(loadLogs, 20000);
  });
})();

