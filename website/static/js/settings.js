// Settings page JS (sin inline scripts)
(function () {
  'use strict';

  const t = (key, fallback) =>
    window.HostBerry?.t ? window.HostBerry.t(key, fallback) : (fallback || key);

  const notify = (message, type = 'info') => {
    const map = { success: 'success', warning: 'warning', error: 'danger', info: 'info' };
    const hbType = map[type] || type;
    if (window.HostBerry?.showAlert) {
      window.HostBerry.showAlert(hbType, message);
      return;
    }
    alert(message);
  };

  const apiRequest = async (url, options) => {
    if (window.HostBerry?.apiRequest) return window.HostBerry.apiRequest(url, options);
    const token = localStorage.getItem('access_token');
    const headers = Object.assign({ 'Content-Type': 'application/json' }, (options && options.headers) || {});
    if (token) headers.Authorization = `Bearer ${token}`;
    return fetch(url, Object.assign({ method: 'GET', headers }, options || {}));
  };

  function bindForm(formId, endpoint) {
    const form = document.getElementById(formId);
    if (!form) return;

    form.addEventListener('submit', async (e) => {
      e.preventDefault();

      // Construir payload robusto (incluye checkboxes aunque estén apagados)
      const obj = {};
      const elements = Array.from(form.elements || []);
      for (const el of elements) {
        if (!el || !el.name || el.disabled) continue;
        const name = el.name;
        const type = (el.type || '').toLowerCase();

        // Omitir DNS primario/secundario y hidden dns_server (lo recomponemos luego)
        if (
          formId === 'networkConfigForm' &&
          (name === 'dns_primary' ||
            name === 'dns_secondary' ||
            (name === 'dns_server' && type === 'hidden'))
        ) {
          continue;
        }

        if (type === 'checkbox') obj[name] = !!el.checked;
        else if (type === 'number') obj[name] = el.value === '' ? null : parseInt(el.value, 10);
        else obj[name] = el.value;
      }

      // Combinar DNS primario + secundario en dns_server
      if (formId === 'networkConfigForm') {
        const dnsPrimary = document.getElementById('dns_primary')?.value?.trim() || '';
        const dnsSecondary = document.getElementById('dns_secondary')?.value?.trim() || '';
        if (dnsPrimary) obj.dns_server = dnsSecondary ? `${dnsPrimary},${dnsSecondary}` : dnsPrimary;
      }

      notify(t('settings.saving', 'Saving...'), 'info');

      try {
        const resp = await apiRequest(endpoint, {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: obj,
        });

        if (!resp || !resp.ok) {
          const errorData = await resp?.json?.().catch(() => ({}));
          const errorMsg =
            errorData?.detail ||
            errorData?.message ||
            t('errors.configuration_error', 'Error saving configuration');
          notify(errorMsg, 'error');
          return;
        }

        const data = await resp.json().catch(() => ({}));
        let message = data.message || t('settings.saved', 'Settings saved successfully');
        const extraErrors = Array.isArray(data.errors) ? data.errors.filter(Boolean) : [];
        if (extraErrors.length && !String(message).includes('Algunos errores')) {
          message += ` (Algunos errores: ${extraErrors.join(', ')})`;
        }
        notify(message, extraErrors.length ? 'warning' : 'success');

        // Aplicar cambios UX
        if (formId === 'generalConfigForm') {
          setTimeout(() => {
            if (obj.language) {
              const currentUrl = new URL(window.location.href);
              currentUrl.searchParams.set('lang', obj.language);
              window.location.href = currentUrl.toString();
              return;
            }

            if (obj.theme && window.ThemeManager) {
              if (obj.theme === 'auto') {
                localStorage.removeItem('theme');
                window.ThemeManager.initTheme?.();
              } else {
                window.ThemeManager.setTheme?.(obj.theme);
              }
            }

            if (obj.timezone) {
              // Guardar localmente para formato de fecha/hora en el frontend
              localStorage.setItem('timezone', String(obj.timezone));
              notify(
                t(
                  'settings.timezone_updated',
                  'Timezone updated. Changes will take effect on next page reload.'
                ),
                'info'
              );
              setTimeout(() => window.location.reload(), 800);
            }
          }, 600);
        }
      } catch (error) {
        console.error('Error saving settings:', error);
        notify(t('errors.network_error', 'Network error. Please try again.'), 'error');
      }
    });
  }

  // Email de prueba
  function bindTestEmail() {
    const testBtn = document.getElementById('sendTestEmailBtn');
    if (!testBtn) return;

    testBtn.addEventListener('click', async () => {
      try {
        const to = (document.getElementById('email_address')?.value || '').trim();
        if (!to) {
          notify(t('settings.test_email_missing_to', 'Please enter an email address first.'), 'warning');
          return;
        }

        notify(t('settings.sending_test_email', 'Sending test email...'), 'info');
        const resp = await apiRequest('/api/v1/system/notifications/test-email', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: { to },
        });

        if (!resp || !resp.ok) {
          const data = await resp?.json?.().catch(() => ({}));
          notify(data?.detail || data?.message || t('settings.test_email_failed', 'Failed to send test email.'), 'error');
          return;
        }

        const data = await resp.json().catch(() => ({}));
        notify(data?.message || t('settings.test_email_sent', 'Test email sent.'), 'success');
      } catch (e) {
        console.error('Test email error:', e);
        notify(t('settings.test_email_failed', 'Failed to send test email.'), 'error');
      }
    });
  }

  const formatUptime = (seconds) =>
    (window.HostBerry && window.HostBerry.formatUptime)
      ? window.HostBerry.formatUptime(seconds)
      : (typeof seconds === 'number' && isFinite(seconds) && seconds >= 0
          ? (() => {
              const d = Math.floor(seconds / 86400);
              const h = Math.floor((seconds % 86400) / 3600);
              const m = Math.floor((seconds % 3600) / 60);
              if (d > 0) return `${d}d ${h}h ${m}m`;
              if (h > 0) return `${h}h ${m}m`;
              return `${m}m`;
            })()
          : '--');

  async function loadSystemInfo() {
    try {
      const [statsResp, infoResp] = await Promise.all([
        apiRequest('/api/v1/system/stats'),
        apiRequest('/api/v1/system/info'),
      ]);

      const stats = statsResp && statsResp.ok ? await statsResp.json().catch(() => ({})) : {};
      const info = infoResp && infoResp.ok ? await infoResp.json().catch(() => ({})) : {};

      const hostname = (info.hostname || stats.hostname || '').trim() || '--';
      const architecture = (info.architecture || stats.architecture || '').trim() || '--';
      const osVersion = (info.os_version || stats.os_version || '').trim() || '--';
      const kernel = (info.kernel_version || info.kernel || stats.kernel_version || stats.kernel || '').trim() || '--';

      let processor = (info.processor || stats.processor || '').trim();
      if (!processor || processor === 'unknown' || processor === 'ARM Processor') {
        processor = info.cpu_model || stats.cpu_model || info.cpu || stats.cpu || '';
      }
      processor = String(processor || '').trim() || '--';

      const uptimeSeconds = info.uptime_seconds || stats.uptime || stats.uptime_seconds || 0;

      const setText = (id, value) => {
        const el = document.getElementById(id);
        if (el) el.textContent = value;
      };

      setText('about-hostname', hostname);
      setText('about-architecture', architecture);
      setText('about-os', osVersion);
      setText('about-kernel', kernel);
      setText('about-processor', processor);

      if (uptimeSeconds !== undefined) {
        setText('about-uptime', formatUptime(Number(uptimeSeconds) || 0));
        const bootTimeEl = document.getElementById('about-boot-time');
        if (bootTimeEl) {
          const bootTimestamp = Math.floor(Date.now() / 1000) - (Number(uptimeSeconds) || 0);
          bootTimeEl.textContent = new Date(bootTimestamp * 1000).toLocaleString();
        }
      }
    } catch (error) {
      console.error('Error loading system info:', error);
    }
  }

  let networkInterfacesData = {};

  function calculateDHCPRange(ip) {
    if (!ip || ip === 'N/A') return null;
    const ipParts = String(ip).split('.');
    if (ipParts.length !== 4) return null;
    const networkBase = ipParts.slice(0, 3).join('.');
    return {
      gateway: `${networkBase}.1`,
      rangeStart: `${networkBase}.100`,
      rangeEnd: `${networkBase}.200`,
    };
  }

  function fillDHCPFields(interfaceName) {
    const iface = networkInterfacesData[interfaceName];
    if (!iface) return;

    const dhcpData = calculateDHCPRange(iface.ip || '');
    const gatewayField = document.getElementById('dhcp_gateway');
    const rangeStartField = document.getElementById('dhcp_range_start');
    const rangeEndField = document.getElementById('dhcp_range_end');

    if (dhcpData) {
      if (gatewayField && !gatewayField.value) gatewayField.value = iface.gateway && iface.gateway !== 'N/A' ? iface.gateway : dhcpData.gateway;
      if (rangeStartField && !rangeStartField.value) rangeStartField.value = dhcpData.rangeStart;
      if (rangeEndField && !rangeEndField.value) rangeEndField.value = dhcpData.rangeEnd;
    } else if (gatewayField && !gatewayField.value && iface.gateway && iface.gateway !== 'N/A') {
      gatewayField.value = iface.gateway;
    }
  }

  async function loadNetworkInterfaces() {
    const select = document.getElementById('dhcp_interface');
    if (!select) return;

    try {
      const resp = await apiRequest('/api/v1/network/interfaces');
      if (!resp || !resp.ok) {
        select.innerHTML = '<option value="">' + t('network.no_interfaces', 'No interfaces found') + '</option>';
        return;
      }

      const result = await resp.json().catch(() => ({}));
      const interfaces = result.interfaces || [];

      networkInterfacesData = {};
      select.innerHTML = '';

      const emptyOption = document.createElement('option');
      emptyOption.value = '';
      emptyOption.textContent = t('settings.select_interface', 'Select interface...');
      select.appendChild(emptyOption);

      interfaces.forEach((iface) => {
        if (!iface || !iface.name) return;
        networkInterfacesData[iface.name] = {
          name: iface.name,
          ip: iface.ip || '',
          mac: iface.mac || '',
          state: iface.state || '',
          gateway: iface.gateway || '',
          netmask: iface.netmask || '',
        };

        const option = document.createElement('option');
        option.value = iface.name;
        option.textContent = iface.ip && iface.ip !== 'N/A' ? `${iface.name} (${iface.ip})` : iface.name;
        select.appendChild(option);
      });
    } catch (error) {
      console.error('Error loading network interfaces:', error);
    }
  }

  function loadDNSValues() {
    const dnsServerValue = document.getElementById('dns_server')?.value;
    const dnsPrimaryInput = document.getElementById('dns_primary');
    const dnsSecondaryInput = document.getElementById('dns_secondary');
    if (!dnsServerValue || !dnsPrimaryInput || !dnsSecondaryInput) return;

    const parts = String(dnsServerValue)
      .split(/[,\s]+/)
      .map((p) => p.trim())
      .filter(Boolean);

    if (parts.length > 0) dnsPrimaryInput.value = parts[0];
    if (parts.length > 1) dnsSecondaryInput.value = parts[1];
  }

  function setupTimezoneSearch() {
    const input = document.getElementById('timezone_search');
    const hidden = document.getElementById('timezone');
    const dropdown = document.getElementById('timezone_dropdown');
    if (!input || !dropdown) return;

    const timezones = [
      'UTC',
      'America/New_York',
      'America/Los_Angeles',
      'America/Chicago',
      'America/Denver',
      'Europe/London',
      'Europe/Paris',
      'Europe/Berlin',
      'Europe/Madrid',
      'Europe/Rome',
      'Asia/Tokyo',
      'Asia/Shanghai',
      'Asia/Singapore',
      'Australia/Sydney',
      'Pacific/Auckland',
    ];

    const renderDropdown = (items) => {
      dropdown.innerHTML = '';
      if (!items.length) {
        const div = document.createElement('div');
        div.className = 'text-white-50 p-2 text-center';
        div.textContent = t('settings.no_results', 'No results');
        dropdown.appendChild(div);
        return;
      }

      items.forEach((tz) => {
        const div = document.createElement('div');
        div.className = 'p-2 text-white';
        div.textContent = tz;
        div.style.cursor = 'pointer';
        div.onmouseover = () => (div.style.backgroundColor = 'rgba(255, 244, 235, 0.08)');
        div.onmouseout = () => (div.style.backgroundColor = 'transparent');
        div.onclick = () => {
          input.value = tz;
          if (hidden) hidden.value = tz;
          dropdown.style.display = 'none';
        };
        dropdown.appendChild(div);
      });
    };

    input.addEventListener('focus', () => {
      renderDropdown(timezones);
      dropdown.style.display = 'block';
    });

    input.addEventListener('input', (e) => {
      const term = String(e.target.value || '').toLowerCase();
      const filtered = timezones.filter((tz) => tz.toLowerCase().includes(term));
      renderDropdown(filtered);
      dropdown.style.display = 'block';
    });

    document.addEventListener('click', (e) => {
      if (!input.contains(e.target) && !dropdown.contains(e.target)) dropdown.style.display = 'none';
    });

    if (hidden && hidden.value) input.value = hidden.value;
  }

  // Init
  document.addEventListener('DOMContentLoaded', () => {
    // Bind forms
    bindForm('generalConfigForm', '/api/v1/system/config');
    bindForm('networkConfigForm', '/api/v1/system/config');
    bindForm('securityConfigForm', '/api/v1/system/config');
    bindForm('performanceConfigForm', '/api/v1/system/config');
    bindForm('notificationConfigForm', '/api/v1/system/config');

    bindTestEmail();

    loadDNSValues();
    setupTimezoneSearch();

    loadSystemInfo();
    setInterval(loadSystemInfo, 30000);

    const select = document.getElementById('dhcp_interface');
    if (select) select.addEventListener('change', (e) => fillDHCPFields(e.target.value));
    loadNetworkInterfaces();
  });
})();

