/**
 * HostBerry - Dark Theme Management
 * Handles switching between light and dark themes
 */

// Function to toggle theme
function toggleTheme() {
    const body = document.body;
    const isDark = body.classList.contains('dark-theme');
    
    if (isDark) {
        body.classList.remove('dark-theme');
        body.classList.add('light-theme');
        localStorage.setItem('theme', 'light');
        updateThemeIcon('light');
    } else {
        body.classList.remove('light-theme');
        body.classList.add('dark-theme');
        localStorage.setItem('theme', 'dark');
        updateThemeIcon('dark');
    }
}

// Function to update theme icon
function updateThemeIcon(theme) {
    const themeToggle = document.getElementById('theme-toggle');
    if (!themeToggle) return;
    const t = (key, fallback) => (window.HostBerry?.t ? window.HostBerry.t(key, fallback) : fallback);
    if (theme === 'dark') {
        themeToggle.textContent = '☀️';
        themeToggle.title = t('common.switch_to_light_theme', 'Switch to light theme');
        themeToggle.classList.remove('light');
        themeToggle.classList.add('dark');
    } else {
        themeToggle.textContent = '🌙';
        themeToggle.title = t('common.switch_to_dark_theme', 'Switch to dark theme');
        themeToggle.classList.remove('dark');
        themeToggle.classList.add('light');
    }
}

// Function to load saved theme
function loadTheme() {
    const savedTheme = localStorage.getItem('theme');
    const body = document.body;
    const serverTheme = window.HostBerryServerSettings && window.HostBerryServerSettings.theme;
    
    // Prioridad: localStorage > settings del servidor > preferencia del sistema
    if (savedTheme === 'dark' || (!savedTheme && serverTheme === 'dark')) {
        body.classList.remove('light-theme');
        body.classList.add('dark-theme');
        updateThemeIcon('dark');
    } else if (savedTheme === 'light' || (!savedTheme && serverTheme === 'light')) {
        body.classList.remove('dark-theme');
        body.classList.add('light-theme');
        updateThemeIcon('light');
    } else {
        // auto / sin setting: usar preferencia del sistema
        if (window.matchMedia('(prefers-color-scheme: dark)').matches) {
            body.classList.remove('light-theme');
            body.classList.add('dark-theme');
            updateThemeIcon('dark');
        } else {
            body.classList.remove('dark-theme');
            body.classList.add('light-theme');
            updateThemeIcon('light');
        }
    }
}

// Function to detect system preference
function detectSystemTheme() {
    const mediaQuery = window.matchMedia('(prefers-color-scheme: dark)');
    
    mediaQuery.addEventListener('change', (e) => {
        if (!localStorage.getItem('theme')) {
            if (e.matches) {
                document.body.classList.remove('light-theme');
                document.body.classList.add('dark-theme');
                updateThemeIcon('dark');
            } else {
                document.body.classList.remove('dark-theme');
                document.body.classList.add('light-theme');
                updateThemeIcon('light');
            }
        }
    });
}

// Function to initialize theme system
function initTheme() {
    loadTheme();
    detectSystemTheme();
    bindThemeToggle();
}

// Bind click handler to theme toggle button (sin onclick inline)
function bindThemeToggle() {
    const themeToggle = document.getElementById('theme-toggle');
    if (!themeToggle) return;
    if (themeToggle.dataset && themeToggle.dataset.hbThemeBound === '1') return;
    if (themeToggle.dataset) themeToggle.dataset.hbThemeBound = '1';
    
    themeToggle.addEventListener('click', function (e) {
        e.preventDefault();
        toggleTheme();
    });
}

// Function to get current theme
function getCurrentTheme() {
    return document.body.classList.contains('dark-theme') ? 'dark' : 'light';
}

// Function to apply specific theme
function setTheme(theme) {
    const body = document.body;
    
    if (theme === 'dark') {
        body.classList.remove('light-theme');
        body.classList.add('dark-theme');
        localStorage.setItem('theme', 'dark');
        updateThemeIcon('dark');
    } else {
        body.classList.remove('dark-theme');
        body.classList.add('light-theme');
        localStorage.setItem('theme', 'light');
        updateThemeIcon('light');
    }
}

// Function to get theme statistics
function getThemeStats() {
    const theme = getCurrentTheme();
    const savedTheme = localStorage.getItem('theme');
    const systemPrefersDark = window.matchMedia('(prefers-color-scheme: dark)').matches;
    
    return {
        current: theme,
        saved: savedTheme,
        systemPrefersDark: systemPrefersDark,
        isAuto: !savedTheme
    };
}

// Initialize when DOM is ready
document.addEventListener('DOMContentLoaded', function() {
    initTheme();
});

// Export functions for global use
window.ThemeManager = {
    toggleTheme,
    loadTheme,
    setTheme,
    getCurrentTheme,
    getThemeStats,
    initTheme
}; 