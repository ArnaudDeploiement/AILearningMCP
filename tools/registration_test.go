// Copyright (c) 2026 Arnaud Guiovanna <https://www.aguiovanna.fr>
// GitHub: https://github.com/ArnaudGuiovanna
// SPDX-License-Identifier: MIT

package tools

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// strippedFrenchStems is a denylist of French stems that have lost their
// diacritics. We only flag French words whose accented form is canonical and
// for which the stripped form has no plausible English collision.
//
// Each entry must match a substring inside a Go string literal taken from a
// `Description:` field (or `jsonschema:` tag). Keep this list small and
// audited — false positives block CI.
var strippedFrenchStems = []string{
	"Determine la",     // "Détermine la" — French phrase
	"Recupere",         // "Récupère"
	"Genere ",          // "Génère " (with trailing space to avoid hitting English "Genere..." ids)
	"activite ",        // "activité "
	"activite.",        // "activité."
	"disponibilite",    // "disponibilité"
	"creneaux",         // "créneaux"
	"duree moyenne",    // "durée moyenne"
	"frequence)",       // "fréquence)"
	"prediction de l'", // "prédiction de l'apprenant" — French only
	"resultat reel",    // "résultat réel"
	"etat cognitif",    // "état cognitif"
	"etat emotionnel",  // "état émotionnel"
	"debut de session", // "début de session"
	"reponse en sec",   // "réponse en secondes"
	"detectees par",    // "détectées par"
	"maitrise",         // "maîtrise" / "maîtrisé"
	"prerequis ",       // "prérequis " (space-bounded to avoid English "prerequisites")
	"prerequis (",      // "prérequis ("
	"prerequis)",       // "prérequis)"
	"prerequis.",       // "prérequis."
	"detruire",         // "détruire" / "détruit"
	"definitivement",   // "définitivement"
	"preserves.",       // "préservés."
	"preservee",        // "préservée"
	"reactiver",        // "réactiver"
	"domaine archive",  // French past participle "domaine archivé"
	"Reactive un",      // French "Réactive un domaine"
	"systeme",          // "système"
	"methode Feynman",  // "méthode Feynman"
	"inedite",          // "inédite"
	"fenetre",          // "fenêtre"
	"caracteres recommande", // "caractères recommandé"
	"Cloture",          // "Clôture"
	"structures pour",  // French "structurés pour"
	"metadonnees",      // "métadonnées"
	"modifies.",        // "modifiés."
	"Necessite",        // "Nécessite"
}

// TestToolDescriptions_NoStrippedFrenchDiacritics walks every tool source file
// and asserts no `Description:` value (or `jsonschema:` tag) contains a known
// stripped-accent French stem. The denylist is conservative: it only flags
// stems that are unambiguously French (no plausible English collision).
func TestToolDescriptions_NoStrippedFrenchDiacritics(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}

	fset := token.NewFileSet()
	var failures []string

	for _, path := range files {
		// Skip test files — denylist literals appear there legitimately.
		if strings.HasSuffix(path, "_test.go") {
			continue
		}

		src, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		f, err := parser.ParseFile(fset, path, src, parser.ParseComments)
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}

		ast.Inspect(f, func(n ast.Node) bool {
			switch x := n.(type) {
			case *ast.KeyValueExpr:
				// Catch `Description: "..."` literals.
				key, ok := x.Key.(*ast.Ident)
				if !ok || key.Name != "Description" {
					return true
				}
				lit, ok := x.Value.(*ast.BasicLit)
				if !ok || lit.Kind != token.STRING {
					return true
				}
				val := lit.Value
				for _, stem := range strippedFrenchStems {
					if strings.Contains(val, stem) {
						pos := fset.Position(lit.Pos())
						failures = append(failures, formatFailure(pos.Filename, pos.Line, stem, val))
					}
				}
			case *ast.BasicLit:
				// Catch raw struct-tag string literals containing
				// `jsonschema:"..."`.
				if x.Kind != token.STRING {
					return true
				}
				val := x.Value
				if !strings.Contains(val, "jsonschema:") {
					return true
				}
				for _, stem := range strippedFrenchStems {
					if strings.Contains(val, stem) {
						pos := fset.Position(x.Pos())
						failures = append(failures, formatFailure(pos.Filename, pos.Line, stem, val))
					}
				}
			}
			return true
		})
	}

	if len(failures) > 0 {
		t.Fatalf("found %d stripped-accent French stem(s) in tool descriptions:\n%s",
			len(failures), strings.Join(failures, "\n"))
	}
}

func formatFailure(file string, line int, stem, val string) string {
	return "  " + file + ":" + itoa(line) + ": stripped stem " + quote(stem) + " in " + truncate(val, 120)
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	neg := i < 0
	if neg {
		i = -i
	}
	var buf [20]byte
	pos := len(buf)
	for i > 0 {
		pos--
		buf[pos] = byte('0' + i%10)
		i /= 10
	}
	if neg {
		pos--
		buf[pos] = '-'
	}
	return string(buf[pos:])
}

func quote(s string) string {
	return "\"" + s + "\""
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
