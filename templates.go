package main

import (
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/template/html/v2"
)

func registerTemplateFuncs(engine *html.Engine) {
	if engine == nil {
		return
	}

	engine.AddFunc("t", func(key string, defaultValue ...string) string {
		if len(defaultValue) > 0 {
			return defaultValue[0]
		}
		return key
	})
	
	engine.AddFunc("json", func(v interface{}) template.HTML {
		b, err := json.Marshal(v)
		if err != nil {
			return template.HTML("{}")
		}
		return template.HTML(b)
	})
	
	engine.AddFunc("eq", func(a, b interface{}) bool {
		return a == b
	})
	
	engine.AddFunc("ne", func(a, b interface{}) bool {
		return a != b
	})
	
	engine.AddFunc("contains", func(s, substr string) bool {
		return strings.Contains(s, substr)
	})

	engine.AddFunc("Seq", func(start, end int) []int {
		var seq []int
		for i := start; i <= end; i++ {
			seq = append(seq, i)
		}
		return seq
	})
}

func createTemplateEngine() *html.Engine {
	var engine *html.Engine
	
	paths := []string{
		"/opt/hostberry/website/templates",  // Ruta de instalación estándar
	}
	
	if wd, err := os.Getwd(); err == nil && wd != "" {
		cur := wd
		for i := 0; i < 6; i++ {
			candidate := filepath.Join(cur, "website", "templates")
			if candidate != "/opt/hostberry/website/templates" {
				paths = append(paths, candidate)
			}
			parent := filepath.Dir(cur)
			if parent == cur {
				break
			}
			cur = parent
		}
	}
	
	exePath, _ := os.Executable()
	if exePath != "" {
		exeDir := filepath.Dir(exePath)
		templatesPath := filepath.Join(exeDir, "website", "templates")
		if templatesPath != "/opt/hostberry/website/templates" {
			paths = append(paths, templatesPath)
		}
	}
	
	paths = append(paths, "./website/templates")
	
	for _, path := range paths {
		if stat, err := os.Stat(path); err == nil {
			if stat.IsDir() {
				if entries, err := os.ReadDir(path); err == nil {
					htmlFiles := 0
					var foundTemplates []string
					for _, entry := range entries {
						if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".html") {
							htmlFiles++
							if len(foundTemplates) < 10 {
								foundTemplates = append(foundTemplates, entry.Name())
							}
						}
					}
						if htmlFiles > 0 {
							log.Printf("✅ %d templates encontrados en %s", htmlFiles, path)
							criticalTemplates := []string{"dashboard.html", "login.html", "base.html", "error.html"}
							missingCritical := false
							for _, tmpl := range criticalTemplates {
								if _, err := os.Stat(filepath.Join(path, tmpl)); err != nil {
									log.Printf("   ⚠️  %s NO encontrado en %s", tmpl, path)
									missingCritical = true
								}
							}
							if missingCritical {
								log.Printf("⚠️  Directorio de templates rechazado por faltantes críticos: %s", path)
								continue
							}

							engine = html.New(path, ".html")
						if engine == nil {
							log.Printf("❌ Error: engine es nil después de html.New para %s", path)
							continue
						}
						
						registerTemplateFuncs(engine)
						
						if err := engine.Load(); err != nil {
							log.Printf("❌ Error cargando templates desde %s: %v", path, err)
							engine = nil
							continue
						}

						log.Printf("✅ Templates cargados desde sistema de archivos: %s", path)
						log.Printf("📊 Total de archivos .html detectados: %d", htmlFiles)
						log.Printf("📝 Lista de templates registrados: %v", foundTemplates)

						engine.Reload(!appConfig.Server.Debug)
						break // Salir del loop, engine encontrado y cargado con éxito
					} else {
						log.Printf("⚠️  Directorio %s existe pero no contiene archivos .html", path)
					}
				}
			}
		}
	}
	
	if engine == nil {
		log.Println("⚠️  Sistema de archivos no disponible, intentando templates embebidos...")
		tmplFS, err := fs.Sub(templatesFS, "website/templates")
		if err == nil {
			if entries, err := fs.ReadDir(tmplFS, "."); err == nil {
				htmlFiles := 0
				var templateNames []string
				for _, entry := range entries {
					if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".html") {
						htmlFiles++
						if len(templateNames) < 5 {
							templateNames = append(templateNames, entry.Name())
						}
					}
				}
				if htmlFiles > 0 {
					criticalTemplates := []string{"dashboard.html", "login.html", "base.html"}
					allCriticalFound := true
					for _, tmpl := range criticalTemplates {
						if testFile, err := tmplFS.Open(tmpl); err == nil {
							testFile.Close()
							log.Printf("   ✅ %s verificado en FS embebido", tmpl)
						} else {
							log.Printf("   ⚠️  No se pudo abrir %s: %v", tmpl, err)
							allCriticalFound = false
						}
					}
					
					if !allCriticalFound {
						log.Printf("⚠️  No todos los templates críticos están disponibles en embebidos, usando fallback")
						err = fmt.Errorf("templates críticos faltantes")
					} else {
						engine = html.NewFileSystem(http.FS(tmplFS), ".html")
						if engine != nil {
							registerTemplateFuncs(engine)
							
							if err := engine.Load(); err != nil {
								log.Printf("❌ Error cargando templates embebidos: %v", err)
								engine = nil
								err = err // para el log de abajo
							} else {
								engine.Reload(false)
								log.Printf("✅ Templates embebidos cargados (MÁS RÁPIDO): %d archivos .html", htmlFiles)
								log.Printf("   Templates encontrados: %v", templateNames)
							}
						} else {
							log.Printf("⚠️  Error: engine es nil después de NewFileSystem con embebidos")
							err = fmt.Errorf("engine es nil")
						}
					}
				} else {
					log.Printf("⚠️  Templates embebidos vacíos, usando fallback")
					err = fmt.Errorf("templates embebidos vacíos")
				}
			} else {
				log.Printf("⚠️  Error leyendo directorio embebido: %v", err)
			}
		} else {
			log.Printf("⚠️  Error creando sub-FS de templates embebidos: %v", err)
			log.Printf("   Intentando acceder directamente al FS...")
			if entries, err := fs.ReadDir(templatesFS, "website/templates"); err == nil {
				htmlFiles := 0
				for _, entry := range entries {
					if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".html") {
						htmlFiles++
					}
				}
				if htmlFiles > 0 {
					log.Printf("✅ Templates encontrados directamente en website/templates: %d archivos", htmlFiles)
					if tmplFS2, err2 := fs.Sub(templatesFS, "website/templates"); err2 == nil {
						engine = html.NewFileSystem(http.FS(tmplFS2), ".html")
						if engine != nil {
							registerTemplateFuncs(engine)
							log.Printf("✅ Motor de templates configurado usando sub-FS directo")
						}
					}
				}
			}
		}
	}
	
	if engine == nil {
		log.Println("⚠️  No se encontró engine después de todos los intentos, forzando desde /opt/hostberry/website/templates")
		forcePath := "/opt/hostberry/website/templates"
		if stat, err := os.Stat(forcePath); err == nil && stat.IsDir() {
			engine = html.New(forcePath, ".html")
			if engine != nil {
				registerTemplateFuncs(engine)
				if err := engine.Load(); err != nil {
					log.Printf("❌ Error cargando templates forzados desde %s: %v", forcePath, err)
					engine = nil
				} else {
					engine.Reload(!appConfig.Server.Debug)
					log.Printf("✅ Engine forzado desde %s", forcePath)
				}
			} else {
				log.Printf("❌ Error: engine es nil después de forzar desde %s", forcePath)
			}
		} else {
			log.Printf("❌ Error: No se pudo acceder a %s: %v", forcePath, err)
		}
	}
	
	if engine == nil {
		log.Fatal("❌ Error crítico: engine es nil después de todos los intentos de carga")
	}
	
	return engine
}

func renderTemplate(c *fiber.Ctx, name string, data fiber.Map) error {
	language := GetCurrentLanguage(c)
	
	i18nFuncs := TemplateFuncs(c)
	
	if data == nil {
		data = fiber.Map{}
	}

	data["page"] = name
	
	data["language"] = language
	data["t"] = i18nFuncs["t"]
	data["common"] = i18nFuncs["common"]
	data["navigation"] = i18nFuncs["navigation"]
	data["dashboard"] = i18nFuncs["dashboard"]
	data["auth"] = i18nFuncs["auth"]
	data["system"] = i18nFuncs["system"]
	data["network"] = i18nFuncs["network"]
	data["wifi"] = i18nFuncs["wifi"]
	data["vpn"] = i18nFuncs["vpn"]
	data["wireguard"] = i18nFuncs["wireguard"]
	data["adblock"] = i18nFuncs["adblock"]
	data["settings"] = i18nFuncs["settings"]
	data["errors"] = i18nFuncs["errors"]
	
	if translations, ok := i18nFuncs["translations"].(map[string]interface{}); ok {
		if translationsJSON, err := json.Marshal(translations); err == nil {
			data["translations"] = translations
			data["translations_json"] = string(translationsJSON)
		}
	}
	
	if user := c.Locals("user"); user != nil {
		data["current_user"] = user
	}
	
	templateName := name
	
	log.Printf("📂 Intentando renderizar template: %s", templateName)

	if err := c.Render(templateName, data, "base"); err == nil {
		return nil
	}

	if err := c.Render(templateName+".html", data, "base"); err == nil {
		return nil
	}
	if err := c.Render(templateName, data, "base.html"); err == nil {
		return nil
	}
	if err := c.Render(templateName+".html", data, "base.html"); err == nil {
		return nil
	}

	if err := c.Render(templateName, data); err == nil {
		return nil
	}

	log.Printf("   ❌ Todos los intentos fallaron para: %s", name)
	if views := c.App().Config().Views; views != nil {
		log.Printf("   ℹ️ Motor de templates está presente")
	} else {
		log.Printf("   ⚠️ Motor de templates NO está configurado")
	}
	return fiber.NewError(500, "Error renderizando template")
}

func copyStaticFiles() error {
	sourceDir := "website/static"
	targetDir := "go-backend/website/static"
	
	if _, err := os.Stat(sourceDir); os.IsNotExist(err) {
		return nil // No hay archivos estáticos que copiar
	}
	
	if err := os.MkdirAll(targetDir, 0755); err != nil {
		return err
	}
	
	return filepath.Walk(sourceDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		
		relPath, err := filepath.Rel(sourceDir, path)
		if err != nil {
			return err
		}
		
		targetPath := filepath.Join(targetDir, relPath)
		
		if info.IsDir() {
			return os.MkdirAll(targetPath, info.Mode())
		}
		
		source, err := os.Open(path)
		if err != nil {
			return err
		}
		defer source.Close()
		
		target, err := os.Create(targetPath)
		if err != nil {
			return err
		}
		defer target.Close()
		
		_, err = io.Copy(target, source)
		return err
	})
}
