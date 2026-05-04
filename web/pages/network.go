package pages

import _ "embed"

//go:embed network.html
var networkHTML string

//go:embed networkScript.js
var networkScript string

func NetworkHTML() string {
	return networkHTML
}

func NetworkScript() string {
	return networkScript
}
