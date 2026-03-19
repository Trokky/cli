package scaffold

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
