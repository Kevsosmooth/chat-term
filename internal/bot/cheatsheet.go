package bot

import (
	"html/template"
	"strings"
)

// cheatsheetHTML renders the one-page cheat sheet from the same tables .help
// uses, so the picture and the help text cannot disagree.
func cheatsheetHTML(p string) string {
	type row struct{ Cmd, What string }
	type section struct {
		Title string
		Rows  []row
	}
	var steps []string
	for _, s := range startSteps {
		steps = append(steps, strings.ReplaceAll(s, "%[1]s", p))
	}
	var sections []section
	for _, g := range groups {
		sec := section{Title: g}
		if g == "Keys" {
			for _, k := range keyCommands {
				sec.Rows = append(sec.Rows, row{p + k.name, k.summary})
			}
		}
		for _, c := range commands {
			if c.group == g && !(g == "Keys" && isKeyCommand(c.name)) {
				sec.Rows = append(sec.Rows, row{usageLine(p, c), c.summary})
			}
		}
		sections = append(sections, sec)
	}
	var sb strings.Builder
	err := cheatsheetTmpl.Execute(&sb, map[string]any{"P": p, "Steps": steps, "Sections": sections})
	if err != nil {
		panic(err) // the template and its data are fixed; a failure is a bug
	}
	return sb.String()
}

var cheatsheetTmpl = template.Must(template.New("cheatsheet").Parse(`<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<title>chat-term cheat sheet</title>
<style>
  * { box-sizing: border-box; margin: 0; padding: 0; }
  body { width: 720px; font: 15px/1.4 -apple-system, "Segoe UI", Roboto, Helvetica, Arial, sans-serif; color: #111827; background: #ffffff; }
  header { background: #111827; color: #ffffff; padding: 18px 24px; }
  h1 { font-size: 24px; }
  header p { margin-top: 4px; color: #d1d5db; }
  main { padding: 16px 24px 24px; }
  h2 { background: #e5e7eb; font-size: 14px; letter-spacing: 0.04em; text-transform: uppercase; padding: 5px 10px; margin: 16px 0 6px; }
  ol { margin: 8px 0 0 22px; }
  li { margin: 4px 0; }
  .start { background: #ecfdf5; border-left: 4px solid #047857; padding: 10px 14px; }
  .start h2 { background: none; padding: 0; margin: 0; color: #047857; }
  table { width: 100%; border-collapse: collapse; }
  td { padding: 3px 10px; vertical-align: top; border-bottom: 1px solid #f3f4f6; }
  td:first-child { width: 42%; }
  code { font: 600 14px/1.4 ui-monospace, Menlo, Consolas, monospace; background: #f3f4f6; padding: 1px 5px; }
  footer { padding: 0 24px 20px; color: #374151; }
  footer p { margin-top: 4px; }
</style>
</head>
<body>
<header>
  <h1>chat-term cheat sheet</h1>
  <p>A message that starts with <b>{{.P}}</b> is a command. Anything else is typed into your terminal, then Enter.</p>
</header>
<main>
  <div class="start">
    <h2>Start here</h2>
    <ol>{{range .Steps}}
      <li>{{.}}</li>{{end}}
    </ol>
  </div>
{{- range .Sections}}
  <h2>{{.Title}}</h2>
  <table>{{range .Rows}}
    <tr><td><code>{{.Cmd}}</code></td><td>{{.What}}</td></tr>{{end}}
  </table>
{{- end}}
</main>
<footer>
  <p>Numbers refer to the last list shown: <code>{{.P}}ls</code> then <code>{{.P}}cat 3</code>.</p>
  <p>Key commands take a repeat count: <code>{{.P}}down 3</code>. To type text that starts with {{.P}}, begin it with <code>{{.P}}{{.P}}</code>.</p>
  <p><code>{{.P}}help &lt;command&gt;</code> explains one command with examples.</p>
</footer>
</body>
</html>
`))
