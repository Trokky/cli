package scaffold

import (
	"encoding/json"
	"fmt"
	"strings"
)

// GeneratePackageJSON generates package.json based on project config.
func GeneratePackageJSON(cfg ProjectConfig) string {
	// Trokky v2 ships the server, mail, i18n and all adapters in the single
	// `trokky` package; adapters are enabled via side-effect imports.
	deps := map[string]string{
		"trokky":         "^2.0.0",
		"@trokky/client": "^2.0.0",
		"express":        "^4.18.2",
		"dotenv":         "^16.3.1",
		"sharp":          "^0.33.0",
	}

	if cfg.DataAdapter == DataPostgres {
		// Declared as an optional dependency of `trokky` for postgres-data.
		deps["pg"] = "^8.11.3"
	}

	if cfg.Studio != StudioNone {
		deps["@trokky/studio"] = "^2.0.0"
	}

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
	bt := "`"

	dataAdapterImport := "import 'trokky/adapters/filesystem-data'"
	if cfg.DataAdapter == DataPostgres {
		dataAdapterImport = "import 'trokky/adapters/postgres-data'"
	}

	// Studio is only mounted when the project embeds or proxies it.
	studioMount := ""
	studioLog := ""
	if cfg.Studio != StudioNone {
		studioMount = "\n      studioPath: '/studio',"
		studioLog = fmt.Sprintf("\n      console.log(%sStudio: http://localhost:${port}${paths.studioPath}%s)", bt, bt)
	}

	// Optional config sections are only forwarded when trokky.config.ts declares
	// them, so the object literal always matches TrokkyConfig.
	extraOptions := "\n\n      // OAuth2 authorization server\n      oauth2: trokkyConfig.oauth2,"
	if cfg.Auth == AuthOAuth {
		extraOptions += "\n\n      // OAuth providers\n      oauth: trokkyConfig.oauth,"
	}
	if cfg.Captcha != CaptchaNone {
		extraOptions += "\n\n      // CAPTCHA verification\n      captcha: trokkyConfig.captcha,"
	}
	if cfg.I18n != I18nNone {
		extraOptions += "\n\n      // Internationalization\n      i18n: trokkyConfig.i18n,"
	}
	if cfg.Mail != MailNone {
		extraOptions += "\n\n      // Transactional email\n      mail: trokkyConfig.mail,"
	}

	return fmt.Sprintf(`import 'dotenv/config'

/**
 * %s - Trokky CMS Server
 */

import express, { type Express } from 'express'
import { TrokkyExpress } from 'trokky/express'
%s
import 'trokky/adapters/filesystem-media'
import trokkyConfig from './trokky.config.js'

async function startServer() {
  const app: Express = express()
  const port: number = Number(process.env.PORT) || 3000

  try {
    const trokky = await TrokkyExpress.create({
      // Content schemas
      schemas: trokkyConfig.schemas,

      // Split storage adapters (data + media)
      storage: trokkyConfig.storage,

      // Media processing and upload rules
      media: trokkyConfig.media,

      // Authentication and security
      security: trokkyConfig.security,

      // HTTP server options
      server: trokkyConfig.server,

      // Studio integration
      studio: trokkyConfig.studio,%s
    })

    trokky.mount(app, {
      apiPath: '/api',%s
    })

    const paths = trokky.getMountedPaths()

    app.get('/health', (_req, res) => {
      res.json({
        status: 'ok',
        timestamp: new Date().toISOString(),
        mountedPaths: paths,
      })
    })

    app.listen(port, () => {
      console.log(%s\n%s running%s)
      console.log(%sServer: http://localhost:${port}%s)
      console.log(%sAPI: http://localhost:${port}${paths.apiPath}%s)%s
      console.log(%sHealth: http://localhost:${port}/health\n%s)
    })
  } catch (error) {
    console.error('Failed to start %s:', error)
    process.exit(1)
  }
}

startServer().catch(error => {
  console.error('Server startup failed:', error)
  process.exit(1)
})
`, cfg.Name, dataAdapterImport, extraOptions, studioMount,
		bt, cfg.Name, bt, bt, bt, bt, bt, studioLog, bt, bt, cfg.Name)
}

// GenerateTrokkyConfig generates the trokky.config.ts file.
func GenerateTrokkyConfig(cfg ProjectConfig) string {
	// Schema imports
	schemaImports := "// import { yourSchema } from './schemas/your-schema.js'"
	schemas := "[\n    // Add your schemas here\n  ] as ContentSchema[]"
	if cfg.IncludeExamples {
		schemaImports = "import { articleSchema } from './schemas/article.js'\nimport { pageSchema } from './schemas/page.js'"
		schemas = "[articleSchema, pageSchema] as ContentSchema[]"
	}

	// Mail is built into `trokky` v2: its adapters live under `trokky/mail/*`,
	// there are no separate @trokky/mail packages.
	mailImport := ""
	mailConsts := ""
	mailConfig := ""
	if cfg.Mail != MailNone {
		mailConsts = fmt.Sprintf(`
const mailFrom = process.env.EMAIL_FROM || 'noreply@example.com'
const mailFromName = process.env.EMAIL_FROM_NAME || '%s'
`, cfg.Name)
		adapter := `new ConsoleMailAdapter({
      from: mailFrom,
      fromName: mailFromName,
      debug: true,
    })`
		mailImport = "import { ConsoleMailAdapter } from 'trokky/mail/console'"
		if cfg.Mail == MailResend {
			mailImport = "import { ResendMailAdapter } from 'trokky/mail/resend'\nimport { ConsoleMailAdapter } from 'trokky/mail/console'"
			adapter = `process.env.RESEND_API_KEY
      ? new ResendMailAdapter({
          apiKey: process.env.RESEND_API_KEY,
          from: mailFrom,
          fromName: mailFromName,
        })
      : new ConsoleMailAdapter({
          from: mailFrom,
          fromName: mailFromName,
          debug: true,
        })`
		}
		mailConfig = fmt.Sprintf(`
  mail: process.env.TROKKY_MAIL_ENABLED === 'true' ? {
    adapter: %s,
    defaultFrom: mailFrom,
    defaultFromName: mailFromName,
  } : undefined,`, adapter)
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
	}

	// Media config
	mediaConfig := `{
      adapter: 'filesystem-media' as const,
      options: {
        mediaDir: './data/media',
        createDirs: true,
      },
    }`

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

import 'dotenv/config'

import type { ContentSchema } from 'trokky'
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
      origin: process.env.PASSKEY_ORIGIN || `+"`"+`http://${process.env.PASSKEY_RP_ID}:3000`+"`"+`,
    } : undefined,
  },

  server: {
    basePath: '',
    port: Number(process.env.PORT) || 3000,
    cors: {
      origin: true,
      credentials: true,
      methods: ['GET', 'POST', 'PUT', 'DELETE', 'OPTIONS', 'PATCH'],
      allowedHeaders: ['Content-Type', 'Authorization', 'X-Requested-With'],
    },
  },

  oauth2: {
    enabled: true,
    issuer: process.env.OAUTH2_ISSUER || 'http://localhost:3000',
  },%s%s%s%s%s
}
`, schemaImports, mailImport, structureImport, mailConsts, schemas,
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
			"target":                           "ES2022",
			"module":                           "NodeNext",
			"moduleResolution":                 "NodeNext",
			"lib":                              []string{"ES2022"},
			"outDir":                           "./dist",
			"rootDir":                          ".",
			"strict":                           true,
			"esModuleInterop":                  true,
			"skipLibCheck":                     true,
			"forceConsistentCasingInFileNames": true,
			"resolveJsonModule":                true,
			"declaration":                      true,
			"declarationMap":                   true,
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

import type { ContentSchema } from 'trokky'

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

import type { ContentSchema } from 'trokky'

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
