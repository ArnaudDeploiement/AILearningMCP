package auth

import (
	"html/template"
	"net/http"
)

type authPageData struct {
	ClientID            string
	RedirectURI         string
	ResponseType        string
	State               string
	CodeChallenge       string
	CodeChallengeMethod string
	Scope               string
}

var authTmpl = template.Must(template.New("auth").Parse(`<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8" />
  <meta name="viewport" content="width=device-width, initial-scale=1.0" />
  <title>Learning Runtime — Sign In</title>
  <style>
    *, *::before, *::after { box-sizing: border-box; margin: 0; padding: 0; }

    body {
      font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif;
      background: #0f1117;
      color: #e2e8f0;
      min-height: 100vh;
      display: flex;
      align-items: center;
      justify-content: center;
    }

    .card {
      background: #1a1d27;
      border: 1px solid #2d3148;
      border-radius: 12px;
      padding: 2.5rem 2rem;
      width: 100%;
      max-width: 420px;
      box-shadow: 0 8px 40px rgba(0,0,0,0.5);
    }

    h1 {
      font-size: 1.4rem;
      font-weight: 600;
      margin-bottom: 0.4rem;
      color: #f8fafc;
    }

    .subtitle {
      font-size: 0.85rem;
      color: #94a3b8;
      margin-bottom: 1.8rem;
    }

    .error-box {
      background: #3b1a1a;
      border: 1px solid #7f1d1d;
      border-radius: 8px;
      padding: 0.75rem 1rem;
      font-size: 0.875rem;
      color: #fca5a5;
      margin-bottom: 1.2rem;
    }

    label {
      display: block;
      font-size: 0.8rem;
      font-weight: 500;
      color: #94a3b8;
      margin-bottom: 0.35rem;
      margin-top: 1rem;
      text-transform: uppercase;
      letter-spacing: 0.05em;
    }

    input[type="email"],
    input[type="password"],
    input[type="text"],
    input[type="url"] {
      width: 100%;
      background: #0f1117;
      border: 1px solid #2d3148;
      border-radius: 8px;
      padding: 0.6rem 0.85rem;
      font-size: 0.95rem;
      color: #e2e8f0;
      outline: none;
      transition: border-color 0.15s;
    }

    input:focus {
      border-color: #6366f1;
    }

    .hint {
      font-size: 0.75rem;
      color: #64748b;
      margin-top: 0.3rem;
    }

    button[type="submit"] {
      margin-top: 1.6rem;
      width: 100%;
      background: #6366f1;
      color: #fff;
      border: none;
      border-radius: 8px;
      padding: 0.7rem;
      font-size: 1rem;
      font-weight: 600;
      cursor: pointer;
      transition: background 0.15s;
    }

    button[type="submit"]:hover {
      background: #4f52cc;
    }

    .footer-note {
      margin-top: 1.2rem;
      font-size: 0.75rem;
      color: #475569;
      text-align: center;
    }
  </style>
</head>
<body>
  <div class="card">
    <h1>Learning Runtime</h1>
    <p class="subtitle">Sign in or create your account to continue.</p>

    {{if .ErrMsg}}
    <div class="error-box">{{.ErrMsg}}</div>
    {{end}}

    <form method="POST" action="/authorize">
      {{/* OAuth hidden fields */}}
      <input type="hidden" name="client_id"             value="{{.Data.ClientID}}" />
      <input type="hidden" name="redirect_uri"          value="{{.Data.RedirectURI}}" />
      <input type="hidden" name="response_type"         value="{{.Data.ResponseType}}" />
      <input type="hidden" name="state"                 value="{{.Data.State}}" />
      <input type="hidden" name="code_challenge"        value="{{.Data.CodeChallenge}}" />
      <input type="hidden" name="code_challenge_method" value="{{.Data.CodeChallengeMethod}}" />
      <input type="hidden" name="scope"                 value="{{.Data.Scope}}" />

      <label for="email">Email</label>
      <input id="email" type="email" name="email" placeholder="you@example.com" required autocomplete="email" />

      <label for="password">Password</label>
      <input id="password" type="password" name="password" placeholder="••••••••" required autocomplete="current-password" />

      <label for="objective">Learning Objective</label>
      <input id="objective" type="text" name="objective" placeholder="e.g. Master Go concurrency patterns" autocomplete="off" />
      <p class="hint">Required for new accounts.</p>

      <label for="webhook_url">Discord Webhook URL</label>
      <input id="webhook_url" type="url" name="webhook_url" placeholder="https://discord.com/api/webhooks/..." autocomplete="off" />
      <p class="hint">Optional — receive reminders in Discord.</p>

      <button type="submit">Continue</button>
    </form>

    <p class="footer-note">By continuing you agree to the Learning Runtime terms of service.</p>
  </div>
</body>
</html>
`))

type tmplData struct {
	Data   authPageData
	ErrMsg string
}

func renderAuthPage(w http.ResponseWriter, data authPageData, errMsg string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if errMsg != "" {
		w.WriteHeader(http.StatusUnauthorized)
	}
	if err := authTmpl.Execute(w, tmplData{Data: data, ErrMsg: errMsg}); err != nil {
		http.Error(w, "template error", http.StatusInternalServerError)
	}
}
