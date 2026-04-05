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
  <title>Learning Runtime</title>
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
      max-width: 400px;
      box-shadow: 0 8px 40px rgba(0,0,0,0.5);
    }

    .logo {
      font-size: 1.5rem;
      font-weight: 700;
      color: #f8fafc;
      margin-bottom: 0.25rem;
    }

    .logo span {
      color: #6366f1;
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

    label:first-of-type {
      margin-top: 0;
    }

    input[type="email"],
    input[type="password"] {
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

    .toggle {
      margin-top: 1.4rem;
      text-align: center;
      font-size: 0.85rem;
      color: #94a3b8;
    }

    .toggle a {
      color: #6366f1;
      text-decoration: none;
      font-weight: 500;
      cursor: pointer;
    }

    .toggle a:hover {
      text-decoration: underline;
    }

    .hidden { display: none; }
  </style>
</head>
<body>
  <div class="card">
    <div class="logo">Learning <span>Runtime</span></div>

    {{if .ErrMsg}}
    <div class="error-box">{{.ErrMsg}}</div>
    {{end}}

    <!-- Login form -->
    <div id="login-view">
      <p class="subtitle">Sign in to continue.</p>
      <form method="POST" action="/authorize">
        <input type="hidden" name="mode" value="login" />
        <input type="hidden" name="client_id"             value="{{.Data.ClientID}}" />
        <input type="hidden" name="redirect_uri"          value="{{.Data.RedirectURI}}" />
        <input type="hidden" name="response_type"         value="{{.Data.ResponseType}}" />
        <input type="hidden" name="state"                 value="{{.Data.State}}" />
        <input type="hidden" name="code_challenge"        value="{{.Data.CodeChallenge}}" />
        <input type="hidden" name="code_challenge_method" value="{{.Data.CodeChallengeMethod}}" />
        <input type="hidden" name="scope"                 value="{{.Data.Scope}}" />

        <label for="login-email">Email</label>
        <input id="login-email" type="email" name="email" placeholder="you@example.com" required autocomplete="email" />

        <label for="login-password">Password</label>
        <input id="login-password" type="password" name="password" placeholder="••••••••" required autocomplete="current-password" />

        <button type="submit">Sign in</button>
      </form>
      <p class="toggle">No account? <a onclick="toggleView()">Create one</a></p>
    </div>

    <!-- Register form -->
    <div id="register-view" class="hidden">
      <p class="subtitle">Create your account.</p>
      <form method="POST" action="/authorize">
        <input type="hidden" name="mode" value="register" />
        <input type="hidden" name="client_id"             value="{{.Data.ClientID}}" />
        <input type="hidden" name="redirect_uri"          value="{{.Data.RedirectURI}}" />
        <input type="hidden" name="response_type"         value="{{.Data.ResponseType}}" />
        <input type="hidden" name="state"                 value="{{.Data.State}}" />
        <input type="hidden" name="code_challenge"        value="{{.Data.CodeChallenge}}" />
        <input type="hidden" name="code_challenge_method" value="{{.Data.CodeChallengeMethod}}" />
        <input type="hidden" name="scope"                 value="{{.Data.Scope}}" />

        <label for="reg-email">Email</label>
        <input id="reg-email" type="email" name="email" placeholder="you@example.com" required autocomplete="email" />

        <label for="reg-password">Password</label>
        <input id="reg-password" type="password" name="password" placeholder="••••••••" required autocomplete="new-password" />

        <label for="reg-confirm">Confirm password</label>
        <input id="reg-confirm" type="password" name="password_confirm" placeholder="••••••••" required autocomplete="new-password" />

        <button type="submit">Create account</button>
      </form>
      <p class="toggle">Already have an account? <a onclick="toggleView()">Sign in</a></p>
    </div>
  </div>

  <script>
    function toggleView() {
      document.getElementById('login-view').classList.toggle('hidden');
      document.getElementById('register-view').classList.toggle('hidden');
    }
    {{if eq .Mode "register"}}toggleView();{{end}}
  </script>
</body>
</html>
`))

type tmplData struct {
	Data   authPageData
	ErrMsg string
	Mode   string // "login" or "register"
}

func renderAuthPage(w http.ResponseWriter, data authPageData, errMsg string, mode string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if errMsg != "" {
		w.WriteHeader(http.StatusUnauthorized)
	}
	if mode == "" {
		mode = "login"
	}
	if err := authTmpl.Execute(w, tmplData{Data: data, ErrMsg: errMsg, Mode: mode}); err != nil {
		http.Error(w, "template error", http.StatusInternalServerError)
	}
}
