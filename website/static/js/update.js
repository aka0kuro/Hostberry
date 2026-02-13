// Update page logic (migrado desde script inline)
(function () {
  const HB = window.HostBerry || {};
  const t = HB.t || window.t || ((key, fallback) => fallback || key);
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

  const setText = (id, value) => {
    const el = document.getElementById(id);
    if (el) el.textContent = value;
  };

  const escapeHtml = (value) => {
    const s = String(value ?? '');
    return HB.escapeHtml
      ? HB.escapeHtml(s)
      : s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;');
  };

  function notify(type, message) {
    if (HB.showAlert) {
      HB.showAlert(type, message);
      return;
    }
    if (window.showAlert) {
      window.showAlert(type, message);
      return;
    }
    alert(message);
  }

  function updateLastCheck() {
    setText('update-last-check', new Date().toLocaleString());
  }

  function addLogEntry(message, type = 'info') {
    const logContainer = document.getElementById('update-log');
    if (!logContainer) return;

    if (logContainer.querySelector('.text-center')) logContainer.innerHTML = '';

    const logEntry = document.createElement('div');
    logEntry.className = `log-item log-${type}`;

    const timestamp = typeof HB.formatTime === 'function'
      ? HB.formatTime(new Date(), { hour: '2-digit', minute: '2-digit', second: '2-digit' })
      : new Date().toLocaleTimeString();

    const icon =
      type === 'success'
        ? 'check-circle'
        : type === 'error' || type === 'danger'
          ? 'x-circle'
          : type === 'warning'
            ? 'exclamation-triangle'
            : 'info-circle';

    const iconColor =
      type === 'success'
        ? 'success'
        : type === 'error' || type === 'danger'
          ? 'danger'
          : type === 'warning'
            ? 'warning'
            : 'info';

    const safeMessage = escapeHtml(message);
    logEntry.innerHTML = `
      <i class="bi bi-${icon} text-${iconColor} me-2"></i>
      <span class="log-time text-white-50">[${timestamp}]</span>
      <span class="log-message">${safeMessage}</span>
    `;

    logContainer.insertBefore(logEntry, logContainer.firstChild);
  }

  function normalizeUpdatesPayload(payload) {
    // Soportar varios formatos (stubs/compat)
    if (Array.isArray(payload)) {
      return {
        updatesAvailable: payload.length > 0,
        updateCount: payload.length,
        updates: payload,
      };
    }

    const updates = payload?.updates || payload?.packages || payload?.list || [];
    const updateCount =
      payload?.update_count ??
      payload?.count ??
      (Array.isArray(updates) ? updates.length : 0);

    const updatesAvailableRaw =
      payload?.updates_available ??
      payload?.available ??
      payload?.available_updates;

    const updatesAvailable =
      typeof updatesAvailableRaw === 'boolean'
        ? updatesAvailableRaw
        : updateCount > 0;

    return { updatesAvailable, updateCount, updates: Array.isArray(updates) ? updates : [] };
  }

  async function checkUpdates() {
    try {
      const list = document.getElementById('system-updates-list');
      if (list) {
        list.innerHTML = `
          <div class="text-center py-4 text-white-50">
            <div class="spinner-border spinner-border-sm me-2" role="status"></div>
            <span>${t('update.checking', 'Checking for updates...')}</span>
          </div>`;
      }

      addLogEntry(t('update.checking', 'Checking for updates...'), 'info');

      const resp = await api('/api/v1/system/updates', { method: 'GET' });
      const payload = await resp?.json?.().catch(() => ({}));

      if (!resp || !resp.ok) {
        throw new Error(payload?.detail || payload?.error || t('update.check_failed', 'Error checking updates'));
      }

      const { updatesAvailable, updateCount, updates } = normalizeUpdatesPayload(payload);
      updateLastCheck();

      if (list) {
        if (updatesAvailable && updateCount > 0) {
          list.innerHTML = `
            <div class="alert alert-warning mb-3">
              <i class="bi bi-exclamation-triangle me-2"></i>
              <strong>${t('update.updates_available', 'Updates available')}:</strong> ${updateCount}
            </div>
            <div class="updates-list logs-container">
              ${updates
                .slice(0, 50)
                .map(
                  (u) =>
                    `<div class="log-item"><i class="bi bi-box-seam text-info me-2"></i><span class="log-message">${escapeHtml(u)}</span></div>`
                )
                .join('')}
            </div>`;
        } else {
          list.innerHTML = `<div class="alert alert-success"><i class="bi bi-check-circle me-2"></i>${t(
            'update.system_updated',
            'System is up to date'
          )}</div>`;
        }
      }

      const message = updatesAvailable
        ? `${t('update.updates_available', 'Updates available')}: ${updateCount}`
        : t('update.system_updated', 'System is up to date');
      addLogEntry(message, updatesAvailable ? 'warning' : 'success');
    } catch (error) {
      const msg = error?.message || t('update.check_failed', 'Error checking updates');
      addLogEntry(msg, 'error');
      notify('danger', msg);
    }
  }

  async function updateSystem() {
    try {
      if (!confirm(t('update.system_confirm', 'Update system?'))) return;
      notify('info', t('update.updating_system', 'Updating system...'));
      addLogEntry(t('update.updating_system', 'Updating system...'), 'info');

      const resp = await api('/api/v1/system/updates/execute', { method: 'POST' });
      if (resp && (resp.status === 404 || resp.status === 405 || resp.status === 501)) {
        const msg = t('errors.not_implemented', 'Not implemented');
        notify('warning', msg);
        addLogEntry(msg, 'warning');
        return;
      }
      const payload = await resp?.json?.().catch(() => ({}));

      if (!resp || !resp.ok || payload?.success === false) {
        throw new Error(payload?.detail || payload?.error || t('update.system_failed', 'Error updating system'));
      }

      notify('success', payload?.message || t('update.system_updated', 'Success'));
      addLogEntry(payload?.message || t('update.system_updated', 'Success'), 'success');
      setTimeout(checkUpdates, 1500);
    } catch (error) {
      const msg = error?.message || t('update.system_failed', 'Error updating system');
      notify('danger', msg);
      addLogEntry(msg, 'error');
    }
  }

  async function updateProject() {
    try {
      if (!confirm(t('update.project_confirm', 'Update project?'))) return;
      notify('info', t('update.updating_project', 'Updating project...'));
      addLogEntry(t('update.updating_project', 'Updating project...'), 'info');

      const resp = await api('/api/v1/system/updates/project', { method: 'POST' });
      if (resp && (resp.status === 404 || resp.status === 405 || resp.status === 501)) {
        const msg = t('errors.not_implemented', 'Not implemented');
        notify('warning', msg);
        addLogEntry(msg, 'warning');
        return;
      }
      const payload = await resp?.json?.().catch(() => ({}));

      if (!resp || !resp.ok || payload?.success === false) {
        throw new Error(payload?.detail || payload?.error || t('update.project_failed', 'Error updating project'));
      }

      notify('success', payload?.message || t('update.project_updated', 'Success'));
      addLogEntry(payload?.message || t('update.project_updated', 'Success'), 'success');
    } catch (error) {
      const msg = error?.message || t('update.project_failed', 'Error updating project');
      notify('danger', msg);
      addLogEntry(msg, 'error');
    }
  }

  async function updateAll() {
    await updateSystem();
    await updateProject();
  }

  async function loadProjectStatus() {
    try {
      const resp = await api('/api/v1/system/stats', { method: 'GET' });
      if (resp && resp.ok) {
        const data = await resp.json().catch(() => ({}));
        setText('project-version', data.version || data.app_version || data.build || '—');
        setText('project-last-update', data.last_update || data.updated_at || data.build_time || '—');
      }
    } catch (_e) {
      // silent
    }
  }

  function initUpdatePage() {
    updateLastCheck();
    loadProjectStatus();

    document.getElementById('action-check-updates')?.addEventListener('click', checkUpdates);
    document.getElementById('action-update-system')?.addEventListener('click', updateSystem);
    document.getElementById('action-update-project')?.addEventListener('click', updateProject);
    document.getElementById('action-update-all')?.addEventListener('click', updateAll);
    document.getElementById('systemUpdatesRefresh')?.addEventListener('click', checkUpdates);
  }

  document.addEventListener('DOMContentLoaded', initUpdatePage);
})();

