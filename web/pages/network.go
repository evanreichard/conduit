package pages

import (
	g "maragu.dev/gomponents"
	h "maragu.dev/gomponents/html"

	_ "embed"
)

//go:embed networkScript.js
var alpineScript string

func NetworkPage() g.Node {
	return h.Doctype(
		h.HTML(
			h.Head(
				h.Script(g.Raw(alpineScript)),
				h.Script(g.Attr("src", "//cdn.tailwindcss.com")),
				h.Script(g.Attr("src", "//cdn.jsdelivr.net/npm/alpinejs@3.x.x/dist/cdn.min.js")),
			),
			h.Body(
				h.Div(h.Class("bg-gray-900 text-gray-100"),
					networkMonitor(),
				),
			),
		),
	)
}

func networkMonitor() g.Node {
	return h.Div(
		g.Attr("x-data", "networkMonitor()"),
		h.Class("h-dvh flex flex-col"),

		// Header
		h.Div(
			h.Class("bg-gray-800 border-b border-gray-700 px-4 py-2 flex items-center gap-4"),
			h.H1(h.Class("text-lg font-semibold"), g.Text("Network")),
			h.Button(
				g.Attr("@click", "clear()"),
				h.Class("px-3 py-1 bg-gray-700 hover:bg-gray-600 rounded text-sm"),
				g.Text("Clear"),
			),
			h.Div(
				h.Class("ml-auto text-sm text-gray-400"),
				h.Span(g.Attr("x-text", "requests.length")),
				g.Text(" requests"),
			),
		),

		// Table
		h.Div(
			h.Class("flex-1 overflow-auto"),
			h.Table(
				h.Class("w-full text-sm"),
				networkTableHeader(),
				networkTableBody(),
			),
		),

		// Details Panel
		networkDetailsPanel(),
	)
}

func networkTableHeader() g.Node {
	return h.THead(
		h.Class("bg-gray-800 sticky top-0 border-b border-gray-700"),
		h.Tr(
			h.Class("text-left"),
			h.Th(h.Class("px-4 py-2 font-medium"), g.Text("Name")),
			h.Th(h.Class("px-4 py-2 font-medium w-20"), g.Text("Method")),
			h.Th(h.Class("px-4 py-2 font-medium w-20"), g.Text("Status")),
			h.Th(h.Class("px-4 py-2 font-medium w-32"), g.Text("Type")),
			h.Th(h.Class("px-4 py-2 font-medium w-32"), g.Text("Time")),
		),
	)
}

func networkTableBody() g.Node {
	return h.TBody(
		h.Template(
			g.Attr("x-for", "req in requests"),
			g.Attr(":key", "req.ID"),
			h.Tr(
				g.Attr("@click", "selected = req"),
				g.Attr(":class", "selected?.ID === req.ID ? 'bg-blue-900' : 'hover:bg-gray-800'"),
				h.Class("border-b border-gray-800 cursor-pointer"),
				h.Td(
					h.Class("px-4 py-2 truncate max-w-md"),
					g.Attr("x-text", "req.URL?.Path || req.URL"),
				),
				h.Td(h.Class("px-4 py-2"), g.Attr("x-text", "req.Method")),
				h.Td(
					h.Class("px-4 py-2"),
					h.Span(
						g.Attr(":class", "statusColor(req.Status)"),
						g.Attr("x-text", "req.Status || '-'"),
					),
				),
				h.Td(
					h.Class("px-4 py-2 text-gray-400"),
					g.Attr("x-text", "req.ResponseBodyType || '-'"),
				),
				h.Td(
					h.Class("px-4 py-2 text-gray-400"),
					g.Attr("x-text", "formatTime(req.Time)"),
				),
			),
		),
	)
}
func networkDetailsPanel() g.Node {
	return h.Div(
		g.Attr("x-show", "selected"),
		g.Attr("x-data", "{ activeTab: 'general' }"),
		h.Class("fixed inset-0 bg-black bg-opacity-50 flex items-center justify-center z-50"),
		g.Attr("@click.self", "selected = null"),
		h.Div(
			h.Class("bg-gray-800 rounded-lg shadow-xl w-3/4 h-3/4 flex flex-col"),
			g.Attr("@click.stop", ""),
			// Header
			h.Div(
				h.Class("flex items-center justify-between p-4 border-b border-gray-700"),
				h.H2(
					h.Class("text-lg font-semibold"),
					g.Attr("x-text", "selected?.URL"),
				),
				h.Button(
					g.Attr("@click", "selected = null"),
					h.Class("text-gray-400 hover:text-gray-200"),
					g.Text("X"),
				),
			),
			// Tabs
			h.Div(
				h.Class("flex border-b border-gray-700"),
				tab("general", "General"),
				tab("request", "Request"),
				tab("response", "Response"),
			),
			// Content
			h.Div(
				h.Class("flex-1 overflow-auto p-4"),
				generalTabContent(),
				requestTabContent(),
				responseTabContent(),
			),
		),
	)
}

func tab(name, label string) g.Node {
	return h.Button(
		g.Attr("@click", "activeTab = '"+name+"'"),
		g.Attr(":class", "activeTab === '"+name+"' ? 'border-b-2 border-blue-500 text-blue-500' : 'text-gray-400 hover:text-gray-200'"),
		h.Class("px-4 py-2 font-medium"),
		g.Text(label),
	)
}

func generalTabContent() g.Node {
	return h.Div(
		g.Attr("x-show", "activeTab === 'general'"),
		h.Class("space-y-4"),
		h.H3(h.Class("font-medium"), g.Text("Details")),
		h.Div(
			h.Class("text-sm space-y-1 text-gray-300"),
			detailRow("URL:", "selected?.URL"),
			detailRow("Source:", "selected?.SourceAddr"),
			detailRow("Method:", "selected?.Method"),
			detailRow("Status:", "selected?.Status"),
		),
	)
}

func requestTabContent() g.Node {
	return h.Div(
		g.Attr("x-show", "activeTab === 'request'"),
		h.Class("space-y-4"),
		h.H3(h.Class("font-medium"), g.Text("Headers")),
		h.Div(
			h.Class("text-sm space-y-1 text-gray-300 font-mono"),
			h.Template(
				g.Attr("x-for", "(values, key) in selected?.RequestHeaders"),
				h.Div(
					h.Span(h.Class("text-gray-500"), g.Attr("x-text", "key + ':'")),
					g.Text(" "),
					h.Span(g.Attr("x-text", "values.join(', ')")),
				),
			),
		),
		h.H3(h.Class("font-medium"), g.Text("Body")),
		h.Pre(
			h.Class("text-sm text-gray-300 font-mono bg-gray-900 p-3 rounded overflow-auto max-h-96"),
			h.Code(g.Attr("x-text", "formatData(selected?.RequestBody)")),
		),
	)
}

func responseTabContent() g.Node {
	return h.Div(
		g.Attr("x-show", "activeTab === 'response'"),
		h.Class("space-y-4"),
		h.H3(h.Class("font-medium"), g.Text("Headers")),
		h.Div(
			h.Class("text-sm space-y-1 text-gray-300 font-mono mb-4"),
			h.Template(
				g.Attr("x-for", "(values, key) in selected?.ResponseHeaders"),
				h.Div(
					h.Span(h.Class("text-gray-500"), g.Attr("x-text", "key + ':'")),
					g.Text(" "),
					h.Span(g.Attr("x-text", "values.join(', ')")),
				),
			),
		),
		h.H3(h.Class("font-medium"), g.Text("Body")),
		h.Pre(
			h.Class("text-sm text-gray-300 font-mono bg-gray-900 p-3 rounded overflow-auto max-h-96"),
			h.Code(g.Attr("x-text", "formatData(selected?.ResponseBody)")),
		),
	)
}

func detailRow(label, value string) g.Node {
	return h.Div(
		h.Span(h.Class("text-gray-500"), g.Text(label)),
		g.Text(" "),
		h.Span(g.Attr("x-text", value)),
	)
}
