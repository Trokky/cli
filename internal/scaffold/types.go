package scaffold

import "fmt"

// Template represents the project template type.
type Template string

const (
	TemplateMinimal Template = "minimal"
	TemplateFull    Template = "full"
	TemplateAPIOnly Template = "api-only"
)

// DataAdapter represents the data storage adapter.
type DataAdapter string

const (
	DataFilesystem DataAdapter = "filesystem"
	DataPostgres   DataAdapter = "postgres"
	DataD1         DataAdapter = "d1"
)

// MediaAdapter represents the media storage adapter.
type MediaAdapter string

const (
	MediaFilesystem MediaAdapter = "filesystem"
	MediaR2         MediaAdapter = "r2"
	MediaS3         MediaAdapter = "s3"
)

// MailProvider represents the mail provider.
type MailProvider string

const (
	MailNone    MailProvider = "none"
	MailResend  MailProvider = "resend"
	MailConsole MailProvider = "console"
)

// AuthMode represents the authentication mode.
type AuthMode string

const (
	AuthBasic AuthMode = "basic"
	AuthOAuth AuthMode = "oauth"
	AuthNone  AuthMode = "none"
)

// StudioMode represents the studio integration mode.
type StudioMode string

const (
	StudioEmbedded StudioMode = "embedded"
	StudioSeparate StudioMode = "separate"
	StudioNone     StudioMode = "none"
)

// CaptchaProvider represents the captcha provider.
type CaptchaProvider string

const (
	CaptchaNone      CaptchaProvider = "none"
	CaptchaTurnstile CaptchaProvider = "turnstile"
	CaptchaRecaptcha CaptchaProvider = "recaptcha"
)

// I18nMode represents the internationalization mode.
type I18nMode string

const (
	I18nNone I18nMode = "none"
	I18nEn   I18nMode = "en"
	I18nFr   I18nMode = "fr"
	I18nEnFr I18nMode = "en-fr"
)

// ProjectConfig holds all configuration for scaffolding a new project.
type ProjectConfig struct {
	Name            string
	Template        Template
	DataAdapter     DataAdapter
	MediaAdapter    MediaAdapter
	Mail            MailProvider
	Auth            AuthMode
	Studio          StudioMode
	Captcha         CaptchaProvider
	I18n            I18nMode
	IncludeExamples bool
}

// Validate checks that all config values are valid.
func (cfg ProjectConfig) Validate() error {
	switch cfg.DataAdapter {
	case DataFilesystem, DataPostgres, DataD1:
	default:
		return fmt.Errorf("invalid data adapter %q (valid: filesystem, postgres, d1)", cfg.DataAdapter)
	}
	switch cfg.MediaAdapter {
	case MediaFilesystem, MediaR2, MediaS3:
	default:
		return fmt.Errorf("invalid media adapter %q (valid: filesystem, r2, s3)", cfg.MediaAdapter)
	}
	switch cfg.Mail {
	case MailNone, MailResend, MailConsole:
	default:
		return fmt.Errorf("invalid mail provider %q (valid: none, resend, console)", cfg.Mail)
	}
	switch cfg.Auth {
	case AuthBasic, AuthOAuth, AuthNone:
	default:
		return fmt.Errorf("invalid auth mode %q (valid: basic, oauth, none)", cfg.Auth)
	}
	switch cfg.Studio {
	case StudioEmbedded, StudioSeparate, StudioNone:
	default:
		return fmt.Errorf("invalid studio mode %q (valid: embedded, separate, none)", cfg.Studio)
	}
	switch cfg.Captcha {
	case CaptchaNone, CaptchaTurnstile, CaptchaRecaptcha:
	default:
		return fmt.Errorf("invalid captcha provider %q (valid: none, turnstile, recaptcha)", cfg.Captcha)
	}
	switch cfg.I18n {
	case I18nNone, I18nEn, I18nFr, I18nEnFr:
	default:
		return fmt.Errorf("invalid i18n mode %q (valid: none, en, fr, en-fr)", cfg.I18n)
	}
	return nil
}

// TemplateDefaults maps template names to their default configs.
var TemplateDefaults = map[Template]ProjectConfig{
	TemplateMinimal: {
		Template:        TemplateMinimal,
		DataAdapter:     DataFilesystem,
		MediaAdapter:    MediaFilesystem,
		Mail:            MailNone,
		Auth:            AuthBasic,
		Studio:          StudioEmbedded,
		Captcha:         CaptchaNone,
		I18n:            I18nNone,
		IncludeExamples: false,
	},
	TemplateFull: {
		Template:        TemplateFull,
		DataAdapter:     DataPostgres,
		MediaAdapter:    MediaS3,
		Mail:            MailResend,
		Auth:            AuthOAuth,
		Studio:          StudioEmbedded,
		Captcha:         CaptchaTurnstile,
		I18n:            I18nEnFr,
		IncludeExamples: true,
	},
	TemplateAPIOnly: {
		Template:        TemplateAPIOnly,
		DataAdapter:     DataFilesystem,
		MediaAdapter:    MediaFilesystem,
		Mail:            MailNone,
		Auth:            AuthBasic,
		Studio:          StudioNone,
		Captcha:         CaptchaNone,
		I18n:            I18nNone,
		IncludeExamples: false,
	},
}
