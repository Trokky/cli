package scaffold

import (
	"encoding/json"
	"fmt"
	"strings"
)

// GeneratePackageJSON generates package.json based on project config.
func GeneratePackageJSON(cfg ProjectConfig) string {
	deps := map[string]string{
		"@trokky/express": "^0.1.14",
		"@trokky/core":    "^0.1.14",
		"@trokky/types":   "^0.1.0",
		"express":         "^4.18.2",
		"dotenv":          "^16.3.1",
	}

	switch cfg.DataAdapter {
	case DataFilesystem:
		deps["@trokky/adapter-filesystem-data"] = "^0.1.1"
	case DataPostgres:
		deps["@trokky/adapter-postgres-data"] = "^0.1.9"
	case DataD1:
		deps["@trokky/adapter-cloudflare-d1"] = "^0.1.0"
	}

	switch cfg.MediaAdapter {
	case MediaFilesystem:
		deps["@trokky/adapter-filesystem-media"] = "^0.1.2"
	case MediaR2:
		deps["@trokky/adapter-cloudflare-r2"] = "^0.1.0"
	case MediaS3:
		deps["@trokky/adapter-s3"] = "^0.1.0"
	}

	switch cfg.Mail {
	case MailResend:
		deps["@trokky/mail"] = "^0.1.3"
		deps["@trokky/mail-adapter-resend"] = "^0.1.0"
		deps["@trokky/mail-adapter-console"] = "^0.1.0"
	case MailConsole:
		deps["@trokky/mail"] = "^0.1.3"
		deps["@trokky/mail-adapter-console"] = "^0.1.0"
	}

	if cfg.Studio == StudioEmbedded || cfg.Studio == StudioSeparate {
		deps["@trokky/studio"] = "^0.1.14"
	}

	if cfg.I18n != I18nNone {
		deps["@trokky/i18n"] = "^0.1.2"
	}

	deps["sharp"] = "^0.33.0"

	pkg := map[string]interface{}{
		"name":    cfg.Name,
		"version": "0.1.0",
		"type":    "module",
		"scripts": map[string]string{
			"dev":   "nodemon",
			"build": "tsc",
			"start": "node dist/server.js",
		},
		"dependencies": deps,
		"devDependencies": map[string]string{
			"@types/express": "^4.17.21",
			"@types/node":    "^20.0.0",
			"typescript":     "^5.3.3",
			"tsx":            "^4.7.0",
			"nodemon":        "^3.0.2",
		},
	}

	data, _ := json.MarshalIndent(pkg, "", "  ")
	return string(data)
}

// GenerateServerTS generates the server.ts entry point.
func GenerateServerTS(cfg ProjectConfig) string {
	var imports []string
	imports = append(imports, "import dotenv from 'dotenv'")
	imports = append(imports, "dotenv.config()")
	imports = append(imports, "")

	switch cfg.DataAdapter {
	case DataFilesystem:
		imports = append(imports, "import '@trokky/adapter-filesystem-data'")
	case DataPostgres:
		imports = append(imports, "import '@trokky/adapter-postgres-data'")
	case DataD1:
		imports = append(imports, "import '@trokky/adapter-cloudflare-d1'")
	}

	switch cfg.MediaAdapter {
	case MediaFilesystem:
		imports = append(imports, "import '@trokky/adapter-filesystem-media'")
	case MediaR2:
		imports = append(imports, "import '@trokky/adapter-cloudflare-r2'")
	case MediaS3:
		imports = append(imports, "import '@trokky/adapter-s3'")
	}

	imports = append(imports, "")
	imports = append(imports, "import { startServer } from '@trokky/express'")
	imports = append(imports, "import config from './trokky.config.js'")

	studioLine := ""
	if cfg.Studio != StudioNone {
		studioLine = fmt.Sprintf("\n  Studio: http://localhost:${info.port}${info.studioPath}")
	}

	return fmt.Sprintf(`/**
 * %s - Trokky CMS Server
 */

%s

const server = await startServer(config)

const info = server.getInfo()
console.log(`+"`"+`
%s running
  API: http://localhost:${info.port}${info.apiPath}%s
`+"`"+`)
`, cfg.Name, strings.Join(imports, "\n"), cfg.Name, studioLine)
}

// GenerateTrokkyConfig generates the trokky.config.ts file.
func GenerateTrokkyConfig(cfg ProjectConfig) string {
	// Schema imports
	schemaImports := "// import { yourSchema } from './schemas/your-schema.js'"
	schemas := "[\n    // Add your schemas here\n  ]"
	if cfg.IncludeExamples {
		schemaImports = "import { articleSchema } from './schemas/article.js'\nimport { pageSchema } from './schemas/page.js'"
		schemas = "[articleSchema, pageSchema]"
	}

	// Mail imports and function
	mailImport := ""
	mailFunction := ""
	mailConfig := ""
	if cfg.Mail == MailResend {
		mailImport = "import { ResendMailAdapter } from '@trokky/mail-adapter-resend'\nimport { ConsoleMailAdapter } from '@trokky/mail-adapter-console'"
		mailFunction = `
function getMailConfig() {
  const enabled = process.env.TROKKY_MAIL_ENABLED === 'true'
  const provider = process.env.TROKKY_MAIL_PROVIDER || 'console'
  if (!enabled) return undefined

  const emailFrom = process.env.EMAIL_FROM || 'noreply@example.com'
  const emailFromName = process.env.EMAIL_FROM_NAME || 'My App'

  if (provider === 'resend' && process.env.RESEND_API_KEY) {
    return {
      adapter: new ResendMailAdapter({
        apiKey: process.env.RESEND_API_KEY,
        from: emailFrom,
        fromName: emailFromName,
        debug: process.env.NODE_ENV === 'development',
      }),
      defaultFrom: emailFrom,
      defaultFromName: emailFromName,
    }
  }

  return {
    adapter: new ConsoleMailAdapter({ from: emailFrom, fromName: emailFromName, debug: true }),
    defaultFrom: emailFrom,
    defaultFromName: emailFromName,
  }
}
`
		mailConfig = "\n  mail: getMailConfig(),"
	} else if cfg.Mail == MailConsole {
		mailImport = "import { ConsoleMailAdapter } from '@trokky/mail-adapter-console'"
		mailFunction = `
function getMailConfig() {
  const enabled = process.env.TROKKY_MAIL_ENABLED === 'true'
  if (!enabled) return undefined

  const emailFrom = process.env.EMAIL_FROM || 'noreply@example.com'
  const emailFromName = process.env.EMAIL_FROM_NAME || 'My App'

  return {
    adapter: new ConsoleMailAdapter({ from: emailFrom, fromName: emailFromName, debug: true }),
    defaultFrom: emailFrom,
    defaultFromName: emailFromName,
  }
}
`
		mailConfig = "\n  mail: getMailConfig(),"
	}

	// Data config
	var dataConfig string
	switch cfg.DataAdapter {
	case DataFilesystem:
		dataConfig = `{
      adapter: 'filesystem-data' as const,
      options: {
        contentDir: './data/content',
        usersDir: './data/users',
        createDirs: true,
      },
    }`
	case DataPostgres:
		dataConfig = `{
      adapter: 'postgres-data' as const,
      options: {
        connection: process.env.DATABASE_URL,
        schema: 'public',
        tablePrefix: 'trokky_',
      },
    }`
	case DataD1:
		dataConfig = `{
      adapter: 'cloudflare-d1' as const,
      options: {
        databaseName: process.env.D1_DATABASE_NAME,
      },
    }`
	}

	// Media config
	var mediaConfig string
	switch cfg.MediaAdapter {
	case MediaFilesystem:
		mediaConfig = `{
      adapter: 'filesystem-media' as const,
      options: {
        mediaDir: './data/media',
        createDirs: true,
      },
    }`
	case MediaR2:
		mediaConfig = `{
      adapter: 'cloudflare-r2' as const,
      options: {
        bucketName: process.env.R2_BUCKET_NAME,
        accountId: process.env.CLOUDFLARE_ACCOUNT_ID,
        accessKeyId: process.env.R2_ACCESS_KEY_ID,
        secretAccessKey: process.env.R2_SECRET_ACCESS_KEY,
      },
    }`
	case MediaS3:
		mediaConfig = `{
      adapter: 's3' as const,
      options: {
        bucket: process.env.S3_BUCKET,
        region: process.env.AWS_REGION,
      },
    }`
	}

	// Captcha config
	captchaConfig := ""
	if cfg.Captcha == CaptchaTurnstile {
		captchaConfig = `
  captcha: process.env.TURNSTILE_SECRET_KEY ? {
    provider: 'turnstile' as const,
    siteKey: process.env.TURNSTILE_SITE_KEY || '',
    secretKey: process.env.TURNSTILE_SECRET_KEY,
  } : undefined,`
	} else if cfg.Captcha == CaptchaRecaptcha {
		captchaConfig = `
  captcha: process.env.RECAPTCHA_SECRET_KEY ? {
    provider: 'recaptcha' as const,
    siteKey: process.env.RECAPTCHA_SITE_KEY || '',
    secretKey: process.env.RECAPTCHA_SECRET_KEY,
  } : undefined,`
	}

	// OAuth config
	oauthConfig := ""
	if cfg.Auth == AuthOAuth {
		oauthConfig = `
  oauth: process.env.GOOGLE_CLIENT_ID ? {
    google: {
      clientId: process.env.GOOGLE_CLIENT_ID,
      clientSecret: process.env.GOOGLE_CLIENT_SECRET || '',
      redirectUri: process.env.GOOGLE_REDIRECT_URI || 'http://localhost:3000/api/auth/google/callback',
    },
  } : undefined,`
	}

	// Studio config
	structureImport := ""
	studioConfig := ""
	if cfg.Studio == StudioEmbedded {
		structureImport = "import { structure } from './structure.js'"
		studioConfig = `
  studio: {
    enabled: true,
    path: '/studio',
    structure,
  },`
	} else if cfg.Studio == StudioSeparate {
		structureImport = "import { structure } from './structure.js'"
		studioConfig = `
  studio: {
    enabled: false,
    apiUrl: process.env.API_URL,
    structure,
  },`
	} else {
		studioConfig = `
  studio: {
    enabled: false,
  },`
	}

	// i18n config
	i18nConfig := ""
	if cfg.I18n != I18nNone {
		defaultLocale := "en"
		if cfg.I18n == I18nFr {
			defaultLocale = "fr"
		}
		supportedLocales := fmt.Sprintf("['%s']", cfg.I18n)
		if cfg.I18n == I18nEnFr {
			supportedLocales = "['en', 'fr']"
		}
		i18nConfig = fmt.Sprintf(`

  i18n: {
    defaultLocale: '%s',
    supportedLocales: %s,
    fallbackLocale: '%s',
    detectBrowserLanguage: true,
  },`, defaultLocale, supportedLocales, defaultLocale)
	}

	return fmt.Sprintf(`/**
 * Trokky Configuration
 */

import dotenv from 'dotenv'
dotenv.config()

%s
%s
%s
%s
export default {
  schemas: %s,

  storage: {
    data: %s,
    media: %s,
  },

  media: {
    processor: 'sharp' as const,
    variants: [
      { name: 'thumbnail', width: 300, height: 200, format: 'webp' as const, quality: 80, fit: 'cover' as const },
      { name: 'medium', width: 800, height: 600, format: 'webp' as const, quality: 85, fit: 'inside' as const },
      { name: 'large', width: 1200, height: 800, format: 'webp' as const, quality: 90, fit: 'cover' as const },
    ],
    upload: {
      maxFileSize: 50 * 1024 * 1024,
      maxFiles: 10,
      allowedMimeTypes: [
        'image/jpeg', 'image/png', 'image/gif', 'image/webp', 'image/svg+xml',
        'application/pdf', 'video/mp4', 'video/webm',
      ],
    },
  },

  security: {
    enabled: true,
    jwtSecret: process.env.JWT_SECRET || 'change-me-in-production',
    adminUser: {
      username: process.env.ADMIN_USERNAME || 'admin',
      email: process.env.ADMIN_EMAIL || 'admin@example.com',
      password: process.env.ADMIN_PASSWORD || 'admin123',
      firstName: 'Admin',
      lastName: 'User',
    },
    passkey: process.env.PASSKEY_RP_ID ? {
      enabled: true,
      rpId: process.env.PASSKEY_RP_ID,
      rpName: process.env.PASSKEY_RP_NAME || '%s',
      origin: process.env.PASSKEY_ORIGIN || ` + "`" + `http://${process.env.PASSKEY_RP_ID}:3000` + "`" + `,
    } : undefined,
  },

  oauth2: {
    enabled: true,
    issuer: process.env.OAUTH2_ISSUER || 'http://localhost:3000',
  },%s%s%s%s%s
}
`, schemaImports, mailImport, structureImport, mailFunction, schemas,
		dataConfig, mediaConfig, cfg.Name,
		oauthConfig, mailConfig, captchaConfig, studioConfig, i18nConfig)
}

// GenerateEnvExample generates the .env.example file.
func GenerateEnvExample(cfg ProjectConfig) string {
	lines := []string{
		"# Server",
		"PORT=3000",
		"NODE_ENV=development",
		"",
		"# Security",
		"JWT_SECRET=your-secret-key-change-in-production",
		"",
		"# Admin User",
		"ADMIN_USERNAME=admin",
		"ADMIN_EMAIL=admin@example.com",
		"ADMIN_PASSWORD=admin123",
	}

	if cfg.Studio != StudioNone {
		lines = append(lines, "", "# Studio", "STUDIO_URL=http://localhost:3000/studio")
	}

	lines = append(lines, "", "# OAuth2 Authorization Server", "OAUTH2_ISSUER=http://localhost:3000")

	if cfg.DataAdapter == DataPostgres {
		lines = append(lines, "", "# Database", "DATABASE_URL=postgres://user:password@localhost:5432/trokky")
	}
	if cfg.DataAdapter == DataD1 {
		lines = append(lines, "", "# Cloudflare D1", "D1_DATABASE_NAME=your-d1-database")
	}

	if cfg.MediaAdapter == MediaR2 {
		lines = append(lines, "", "# Cloudflare R2",
			"CLOUDFLARE_ACCOUNT_ID=your-account-id",
			"R2_BUCKET_NAME=your-bucket",
			"R2_ACCESS_KEY_ID=your-access-key",
			"R2_SECRET_ACCESS_KEY=your-secret-key")
	}
	if cfg.MediaAdapter == MediaS3 {
		lines = append(lines, "", "# AWS S3",
			"AWS_REGION=us-east-1",
			"S3_BUCKET=your-bucket",
			"AWS_ACCESS_KEY_ID=your-access-key",
			"AWS_SECRET_ACCESS_KEY=your-secret-key")
	}

	if cfg.Mail != MailNone {
		lines = append(lines, "", "# Email",
			"TROKKY_MAIL_ENABLED=true",
			fmt.Sprintf("TROKKY_MAIL_PROVIDER=%s", cfg.Mail))
		if cfg.Mail == MailResend {
			lines = append(lines, "RESEND_API_KEY=re_xxxxx")
		}
		lines = append(lines, "EMAIL_FROM=noreply@example.com", "EMAIL_FROM_NAME=My App")
	}

	if cfg.Auth == AuthOAuth {
		lines = append(lines, "", "# OAuth (Google)",
			"GOOGLE_CLIENT_ID=your-client-id",
			"GOOGLE_CLIENT_SECRET=your-client-secret",
			"GOOGLE_REDIRECT_URI=http://localhost:3000/api/auth/google/callback")
	}

	if cfg.Captcha == CaptchaTurnstile {
		lines = append(lines, "", "# Cloudflare Turnstile",
			"TURNSTILE_SITE_KEY=your-site-key",
			"TURNSTILE_SECRET_KEY=your-secret-key")
	}
	if cfg.Captcha == CaptchaRecaptcha {
		lines = append(lines, "", "# Google reCAPTCHA",
			"RECAPTCHA_SITE_KEY=your-site-key",
			"RECAPTCHA_SECRET_KEY=your-secret-key")
	}

	lines = append(lines, "",
		"# Passkey/WebAuthn (optional)",
		"# PASSKEY_RP_ID=localhost",
		"# PASSKEY_RP_NAME=My CMS",
		"# PASSKEY_ORIGIN=http://localhost:3000")

	return strings.Join(lines, "\n") + "\n"
}

// GenerateTsConfig generates tsconfig.json.
func GenerateTsConfig() string {
	cfg := map[string]interface{}{
		"compilerOptions": map[string]interface{}{
			"target":                         "ES2022",
			"module":                         "NodeNext",
			"moduleResolution":               "NodeNext",
			"lib":                            []string{"ES2022"},
			"outDir":                         "./dist",
			"rootDir":                        ".",
			"strict":                         true,
			"esModuleInterop":                true,
			"skipLibCheck":                   true,
			"forceConsistentCasingInFileNames": true,
			"resolveJsonModule":              true,
			"declaration":                    true,
			"declarationMap":                 true,
		},
		"include": []string{"*.ts", "schemas/**/*"},
		"exclude": []string{"node_modules", "dist"},
	}
	data, _ := json.MarshalIndent(cfg, "", "  ")
	return string(data) + "\n"
}

// GenerateNodemonConfig generates nodemon.json.
func GenerateNodemonConfig() string {
	cfg := map[string]interface{}{
		"watch":  []string{"."},
		"ignore": []string{"data/**", "dist/**", "*.log"},
		"ext":    "ts,js",
		"exec":   "tsx server.ts",
	}
	data, _ := json.MarshalIndent(cfg, "", "  ")
	return string(data) + "\n"
}

// GenerateGitignore generates .gitignore.
func GenerateGitignore() string {
	return `# Dependencies
node_modules/

# Build
dist/

# Data (local development)
data/

# Environment
.env
.env.local

# Logs
*.log
npm-debug.log*

# IDE
.idea/
.vscode/
*.swp
*.swo

# OS
.DS_Store
Thumbs.db
`
}

// GenerateNpmrc generates .npmrc for GitHub Packages.
func GenerateNpmrc() string {
	return `@trokky:registry=https://npm.pkg.github.com
//npm.pkg.github.com/:_authToken=${NODE_AUTH_TOKEN}
`
}

// GenerateStructureTS generates the Studio structure.ts file.
func GenerateStructureTS(cfg ProjectConfig) string {
	if cfg.IncludeExamples {
		return fmt.Sprintf(`/**
 * %s Structure Configuration
 * Defines how content types appear in the Studio sidebar
 */

export const structure = async (context: any) => {
  return {
    title: '%s',
    items: [
      { type: 'documentList', title: 'Articles', schemaType: 'article', icon: 'FaNewspaper' },
      { type: 'divider' },
      { type: 'documentList', title: 'Pages', schemaType: 'page', icon: 'FaFile' },
    ],
  }
}
`, cfg.Name, cfg.Name)
	}

	return fmt.Sprintf(`/**
 * %s Structure Configuration
 * Defines how content types appear in the Studio sidebar
 */

export const structure = async (context: any) => {
  const { schemas } = context

  const items = schemas.map((schema: any) => ({
    type: schema.type === 'singleton' ? 'singleton' : 'documentList',
    title: schema.title,
    schemaType: schema.name,
    ...(schema.type === 'singleton' ? { documentId: schema.name } : {}),
  }))

  return {
    title: '%s',
    items,
  }
}
`, cfg.Name, cfg.Name)
}

// GenerateExampleArticleSchema generates an example article schema.
func GenerateExampleArticleSchema() string {
	return `/**
 * Article Schema - Example content type
 */

import type { ContentSchema } from '@trokky/types'

export const articleSchema: ContentSchema = {
  name: 'article',
  title: 'Article',
  type: 'document',
  fields: [
    { name: 'title', title: 'Title', type: 'string', required: true },
    { name: 'slug', title: 'Slug', type: 'slug', options: { source: 'title' } },
    { name: 'content', title: 'Content', type: 'richtext' },
    { name: 'featuredImage', title: 'Featured Image', type: 'media', options: { accept: 'image/*' } },
    { name: 'publishedAt', title: 'Published At', type: 'datetime' },
  ],
}
`
}

// GenerateExamplePageSchema generates an example page schema.
func GenerateExamplePageSchema() string {
	return `/**
 * Page Schema - Example content type
 */

import type { ContentSchema } from '@trokky/types'

export const pageSchema: ContentSchema = {
  name: 'page',
  title: 'Page',
  type: 'document',
  fields: [
    { name: 'title', title: 'Title', type: 'string', required: true },
    { name: 'slug', title: 'Slug', type: 'slug', options: { source: 'title' } },
    { name: 'content', title: 'Content', type: 'richtext' },
  ],
}
`
}

// GenerateSchemaIndex generates schemas/index.ts barrel export.
func GenerateSchemaIndex(cfg ProjectConfig) string {
	if cfg.IncludeExamples {
		return `import { articleSchema } from './article.js'
import { pageSchema } from './page.js'

export const schemas = [articleSchema, pageSchema]
`
	}
	return `// Import your schemas here
// import { yourSchema } from './your-schema.js'

export const schemas = [
  // Add your schemas here
]
`
}
