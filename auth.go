package main

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

// LoginError lleva una clave i18n para que el handler traduzca el mensaje según el idioma.
type LoginError struct {
	Key     string        // ej: "auth.invalid_credentials"
	Default string        // mensaje por defecto (español)
	Args    []interface{} // argumentos para reemplazar {minutes}, {duration}, etc.
}

func (e *LoginError) Error() string { return e.Default }

type Claims struct {
	Username string `json:"username"`
	UserID   int    `json:"user_id"`
	jwt.RegisteredClaims
}

type User struct {
	ID        int    `gorm:"primaryKey"`
	Username  string `gorm:"unique;not null"`
	Password  string `gorm:"not null"`
	Email     string
	FirstName string
	LastName  string
	Role      string `gorm:"default:admin"`
	Timezone  string `gorm:"default:UTC"`

	LastLogin          *time.Time
	LoginCount         int       `gorm:"default:0"`
	FailedAttempts     int       `gorm:"default:0"`
	LockedUntil        *time.Time // desbloqueo automático tras lockout
	EmailNotifications bool `gorm:"default:false"`
	SystemAlerts       bool `gorm:"default:false"`
	SecurityAlerts     bool `gorm:"default:false"`
	ShowActivity       bool `gorm:"default:true"`
	DataCollection     bool `gorm:"default:false"`
	Analytics          bool `gorm:"default:false"`

	IsActive  bool `gorm:"default:true"`
	CreatedAt time.Time
	UpdatedAt time.Time
}

func GenerateToken(user *User) (string, error) {
	expirationTime := time.Now().Add(time.Duration(appConfig.Security.TokenExpiry) * time.Minute)

	claims := &Claims{
		Username: user.Username,
		UserID:   user.ID,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(expirationTime),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			NotBefore: jwt.NewNumericDate(time.Now()),
			Issuer:    "hostberry",
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(appConfig.Security.JWTSecret))
}

func ValidateToken(tokenString string) (*Claims, error) {
	if tokenString == "" {
		return nil, errors.New("token vacío")
	}

	claims := &Claims{}

	token, err := jwt.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("método de firma inválido")
		}
		return []byte(appConfig.Security.JWTSecret), nil
	})

	if err != nil {
		errMsg := strings.ToLower(err.Error())
		if strings.Contains(errMsg, "expired") || strings.Contains(errMsg, "token is expired") {
			return nil, errors.New("token expirado")
		}
		if strings.Contains(errMsg, "not valid yet") || strings.Contains(errMsg, "token is not valid yet") {
			return nil, errors.New("token aún no válido")
		}
		if strings.Contains(errMsg, "malformed") || strings.Contains(errMsg, "token is malformed") {
			return nil, errors.New("token malformado")
		}
		return nil, fmt.Errorf("error validando token: %v", err)
	}

	if !token.Valid {
		return nil, errors.New("token inválido")
	}

	if claims.ExpiresAt != nil && claims.ExpiresAt.Time.Before(time.Now()) {
		return nil, errors.New("token expirado")
	}

	return claims, nil
}

func HashPassword(password string) (string, error) {
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), appConfig.Security.BcryptCost)
	return string(bytes), err
}

func isBcryptHash(hash string) bool {
	return strings.HasPrefix(hash, "$2a$") || strings.HasPrefix(hash, "$2b$") || strings.HasPrefix(hash, "$2y$")
}

func CheckPassword(password, hash string) bool {
	if !isBcryptHash(hash) {
		return password == hash
	}
	normalized := hash
	if strings.HasPrefix(hash, "$2y$") {
		normalized = "$2a$" + strings.TrimPrefix(hash, "$2y$")
	}
	return bcrypt.CompareHashAndPassword([]byte(normalized), []byte(password)) == nil
}

func getMaxLoginAttempts() int {
	defaultAttempts := 3
	raw, err := GetConfig("max_login_attempts")
	if err != nil {
		return defaultAttempts
	}
	v, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || v <= 0 || v > 20 {
		return defaultAttempts
	}
	return v
}

func getLockoutMinutes() int {
	if appConfig.Security.LockoutMinutes > 0 {
		return appConfig.Security.LockoutMinutes
	}
	return 15
}

func Login(username, password string) (*User, string, error) {
	var user User
	if err := db.Where("username = ? AND is_active = ?", username, true).First(&user).Error; err != nil {
		return nil, "", &LoginError{Key: "auth.invalid_credentials", Default: "usuario o contraseña incorrectos"}
	}

	maxAttempts := getMaxLoginAttempts()
	lockoutMin := getLockoutMinutes()
	now := time.Now()

	if user.FailedAttempts >= maxAttempts {
		if user.LockedUntil != nil && now.Before(*user.LockedUntil) {
			remaining := time.Until(*user.LockedUntil).Round(time.Second)
			return nil, "", &LoginError{Key: "auth.account_locked_retry_in", Default: "cuenta bloqueada. Podrás intentar de nuevo en " + remaining.String(), Args: []interface{}{remaining.String()}}
		}
		// Desbloqueo automático: ya pasó el tiempo
		user.FailedAttempts = 0
		user.LockedUntil = nil
		_ = db.Save(&user).Error
	}

	if !CheckPassword(password, user.Password) {
		user.FailedAttempts++
		if user.FailedAttempts >= maxAttempts && lockoutMin > 0 {
			until := now.Add(time.Duration(lockoutMin) * time.Minute)
			user.LockedUntil = &until
		}
		_ = db.Save(&user).Error
		if user.FailedAttempts >= maxAttempts {
			if user.LockedUntil != nil {
				return nil, "", &LoginError{Key: "auth.too_many_attempts_time", Default: "demasiados intentos fallidos. Cuenta bloqueada " + strconv.Itoa(lockoutMin) + " minutos.", Args: []interface{}{lockoutMin}}
			}
			return nil, "", &LoginError{Key: "auth.too_many_attempts", Default: "demasiados intentos fallidos. Intenta nuevamente más tarde"}
		}
		return nil, "", &LoginError{Key: "auth.invalid_credentials", Default: "usuario o contraseña incorrectos"}
	}

	if !strings.HasPrefix(user.Password, "$2a$") && !strings.HasPrefix(user.Password, "$2b$") {
		if hashed, err := HashPassword(password); err == nil {
			user.Password = hashed
			_ = db.Save(&user).Error
		}
	}

	user.FailedAttempts = 0
	user.LockedUntil = nil
	user.LastLogin = &now
	user.LoginCount++
	_ = db.Save(&user).Error

	token, err := GenerateToken(&user)
	if err != nil {
		return nil, "", err
	}

	return &user, token, nil
}

func Register(username, password, email string) (*User, error) {
	if username == "" {
		return nil, errors.New("el nombre de usuario no puede estar vacío")
	}
	if err := ValidateUsername(username); err != nil {
		return nil, err
	}

	if password == "" {
		return nil, errors.New("la contraseña no puede estar vacía")
	}
	if err := ValidatePassword(password); err != nil {
		return nil, err
	}
	if err := ValidateEmail(email); err != nil {
		return nil, err
	}

	var existingUser User
	if err := db.Where("username = ?", username).First(&existingUser).Error; err == nil {
		return nil, errors.New("el usuario ya existe")
	}

	hashedPassword, err := HashPassword(password)
	if err != nil {
		return nil, fmt.Errorf("error hasheando contraseña: %v", err)
	}

	user := User{
		Username: username,
		Password: hashedPassword,
		Email:    email,
		Role:     "admin",
		Timezone: "UTC",
		IsActive: true,
	}

	if err := db.Create(&user).Error; err != nil {
		return nil, fmt.Errorf("error creando usuario en BD: %v", err)
	}

	return &user, nil
}

// RegisterBootstrap crea un usuario sin validar fortaleza de contraseña (solo para admin inicial).
func RegisterBootstrap(username, password, email string) (*User, error) {
	if username == "" || password == "" {
		return nil, errors.New("usuario y contraseña no pueden estar vacíos")
	}
	if err := ValidateUsername(username); err != nil {
		return nil, err
	}
	var existingUser User
	if err := db.Where("username = ?", username).First(&existingUser).Error; err == nil {
		return nil, errors.New("el usuario ya existe")
	}
	hashedPassword, err := HashPassword(password)
	if err != nil {
		return nil, fmt.Errorf("error hasheando contraseña: %v", err)
	}
	user := User{
		Username: username,
		Password: hashedPassword,
		Email:    email,
		Role:     "admin",
		Timezone: "UTC",
		IsActive: true,
	}
	if err := db.Create(&user).Error; err != nil {
		return nil, fmt.Errorf("error creando usuario en BD: %v", err)
	}
	return &user, nil
}
