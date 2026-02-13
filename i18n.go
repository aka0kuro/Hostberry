package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/gofiber/fiber/v2"
)

type I18nManager struct {
	translations     map[string]map[string]interface{}
	defaultLanguage  string
	supportedLanguages []string
	mu               sync.RWMutex
}

var i18nManager *I18nManager
var logLanguage string = "es" // Idioma por defecto para logs del sistema

func InitI18n(localesPath string) error {
	i18nManager = &I18nManager{
		translations:      make(map[string]map[string]interface{}),
		defaultLanguage:   "es",
		supportedLanguages: []string{"es", "en"},
	}

	paths := []string{
		localesPath,
		"locales",
		"/opt/hostberry/locales",
		filepath.Join(filepath.Dir(os.Args[0]), "locales"),
	}

	var foundPath string
	for _, path := range paths {
		if _, err := os.Stat(path); err == nil {
			foundPath = path
			break
		}
	}

	if foundPath == "" {
		return fmt.Errorf("directorio de locales no encontrado")
	}

	for _, lang := range i18nManager.supportedLanguages {
		langFile := filepath.Join(foundPath, lang+".json")
		if err := i18nManager.loadLanguage(lang, langFile); err != nil {
			fmt.Printf("⚠️  Advertencia: No se pudo cargar %s: %v\n", langFile, err)
		}
	}

	return nil
}

func (i *I18nManager) loadLanguage(lang, filePath string) error {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return err
	}

	var translations map[string]interface{}
	if err := json.Unmarshal(data, &translations); err != nil {
		return err
	}

	i.mu.Lock()
	i.translations[lang] = translations
	i.mu.Unlock()

	return nil
}

func (i *I18nManager) GetText(key, language string, defaultValue string) string {
	if language == "" {
		language = i.defaultLanguage
	}

	i.mu.RLock()
	defer i.mu.RUnlock()

	langTranslations, ok := i.translations[language]
	if !ok {
		langTranslations = i.translations[i.defaultLanguage]
	}

	value := i.getNestedValue(langTranslations, key)
	if value != "" {
		return value
	}

	if language != i.defaultLanguage {
		defaultTranslations := i.translations[i.defaultLanguage]
		value = i.getNestedValue(defaultTranslations, key)
		if value != "" {
			return value
		}
	}

	if defaultValue != "" {
		return defaultValue
	}
	return key
}

func (i *I18nManager) getNestedValue(data map[string]interface{}, key string) string {
	keys := strings.Split(key, ".")
	current := data

	for i, k := range keys {
		if val, ok := current[k]; ok {
			if i == len(keys)-1 {
				if str, ok := val.(string); ok {
					return str
				}
				return ""
			}
			if nested, ok := val.(map[string]interface{}); ok {
				current = nested
			} else {
				return ""
			}
		} else {
			return ""
		}
	}

	return ""
}

func (i *I18nManager) GetTranslations(language string) map[string]interface{} {
	if language == "" {
		language = i.defaultLanguage
	}

	i.mu.RLock()
	defer i.mu.RUnlock()

	translations, ok := i.translations[language]
	if !ok {
		translations = i.translations[i.defaultLanguage]
	}

	return translations
}

func GetCurrentLanguage(c *fiber.Ctx) string {
	if i18nManager == nil {
		return "es" // Fallback seguro
	}

	if lang := c.Query("lang"); lang != "" {
		if isLanguageSupported(lang) {
			c.Cookie(&fiber.Cookie{
				Name:     "lang",
				Value:    lang,
				Path:     "/",
				MaxAge:   365 * 24 * 60 * 60, // 1 año
				HTTPOnly: false, // Permitir acceso desde JavaScript
				SameSite: "Lax",
			})
			return lang
		}
	}

	if lang := c.Cookies("lang"); lang != "" {
		if isLanguageSupported(lang) {
			return lang
		}
	}

	acceptLang := c.Get("Accept-Language", "")
	if acceptLang != "" {
		langs := strings.Split(acceptLang, ",")
		if len(langs) > 0 {
			lang := strings.TrimSpace(strings.Split(langs[0], ";")[0])
			if len(lang) >= 2 {
				lang = lang[:2]
				if isLanguageSupported(lang) {
					return lang
				}
			}
		}
	}

	return i18nManager.defaultLanguage
}

func isLanguageSupported(lang string) bool {
	if i18nManager == nil {
		return lang == "es" || lang == "en" // Fallback seguro
	}
	for _, supported := range i18nManager.supportedLanguages {
		if lang == supported {
			return true
		}
	}
	return false
}

func T(c *fiber.Ctx, key string, defaultValue string) string {
	if i18nManager == nil {
		if defaultValue != "" {
			return defaultValue
		}
		return key
	}
	language := GetCurrentLanguage(c)
	return i18nManager.GetText(key, language, defaultValue)
}

func TemplateFuncs(c *fiber.Ctx) fiber.Map {
	language := GetCurrentLanguage(c)
	
	if i18nManager == nil {
		return fiber.Map{
			"t": func(key string, defaultValue ...string) string {
				if len(defaultValue) > 0 {
					return defaultValue[0]
				}
				return key
			},
			"language": language,
			"translations": make(map[string]interface{}),
			"common": make(map[string]interface{}),
			"navigation": make(map[string]interface{}),
			"dashboard": make(map[string]interface{}),
			"auth": make(map[string]interface{}),
			"system": make(map[string]interface{}),
			"network": make(map[string]interface{}),
			"wifi": make(map[string]interface{}),
			"vpn": make(map[string]interface{}),
			"wireguard": make(map[string]interface{}),
			"adblock": make(map[string]interface{}),
			"settings": make(map[string]interface{}),
			"errors": make(map[string]interface{}),
		}
	}

	translations := i18nManager.GetTranslations(language)

	return fiber.Map{
		"t": func(key string, defaultValue ...string) string {
			def := ""
			if len(defaultValue) > 0 {
				def = defaultValue[0]
			}
			return i18nManager.GetText(key, language, def)
		},
		"language": language,
		"translations": translations,
		"common": getSection(translations, "common"),
		"navigation": getSection(translations, "navigation"),
		"dashboard": getSection(translations, "dashboard"),
		"auth": getSection(translations, "auth"),
		"system": getSection(translations, "system"),
		"network": getSection(translations, "network"),
		"wifi": getSection(translations, "wifi"),
		"vpn": getSection(translations, "vpn"),
		"wireguard": getSection(translations, "wireguard"),
		"adblock": getSection(translations, "adblock"),
		"settings": getSection(translations, "settings"),
		"errors": getSection(translations, "errors"),
	}
}

func getSection(translations map[string]interface{}, section string) map[string]interface{} {
	if val, ok := translations[section]; ok {
		if sectionMap, ok := val.(map[string]interface{}); ok {
			return sectionMap
		}
	}
	return make(map[string]interface{})
}

// SetLogLanguage establece el idioma para los logs del sistema
func SetLogLanguage(lang string) {
	if isLanguageSupported(lang) {
		logLanguage = lang
	}
}

// GetLogLanguage obtiene el idioma actual para logs
func GetLogLanguage() string {
	return logLanguage
}

// LogT traduce y registra un mensaje de log
func LogT(key string, args ...interface{}) {
	if i18nManager == nil {
		// Fallback: no asumir que "key" es un format string
		if len(args) > 0 {
			log.Print(append([]interface{}{key}, args...)...)
			return
		}
		log.Print(key)
		return
	}
	
	translated := i18nManager.GetText(key, logLanguage, key)
	if len(args) > 0 {
		log.Print(fmt.Sprintf(translated, args...))
		return
	}
	log.Print(translated)
}

// LogTf traduce y registra un mensaje de log con formato
func LogTf(key string, args ...interface{}) {
	if i18nManager == nil {
		// Fallback: no asumir que "key" es un format string
		if len(args) > 0 {
			log.Print(append([]interface{}{key}, args...)...)
			return
		}
		log.Print(key)
		return
	}
	
	translated := i18nManager.GetText(key, logLanguage, key)
	if len(args) > 0 {
		log.Print(fmt.Sprintf(translated, args...))
		return
	}
	log.Print(translated)
}

// LogTln traduce y registra un mensaje de log con nueva línea
func LogTln(key string, args ...interface{}) {
	if i18nManager == nil {
		// Fallback: no asumir que "key" es un format string
		if len(args) > 0 {
			log.Println(append([]interface{}{key}, args...)...)
			return
		}
		log.Println(key)
		return
	}
	
	translated := i18nManager.GetText(key, logLanguage, key)
	if len(args) > 0 {
		log.Println(fmt.Sprintf(translated, args...))
		return
	}
	log.Println(translated)
}

// LogTfatal traduce y registra un mensaje fatal
func LogTfatal(key string, args ...interface{}) {
	if i18nManager == nil {
		// Fallback: no asumir que "key" es un format string
		if len(args) > 0 {
			log.Fatal(append([]interface{}{key}, args...)...)
			return
		}
		log.Fatal(key)
		return
	}
	
	translated := i18nManager.GetText(key, logLanguage, key)
	if len(args) > 0 {
		log.Fatal(fmt.Sprintf(translated, args...))
		return
	}
	log.Fatal(translated)
}

func LanguageMiddleware(c *fiber.Ctx) error {
	language := GetCurrentLanguage(c)
	c.Locals("language", language)
	c.Locals("i18n", TemplateFuncs(c))
	return c.Next()
}
