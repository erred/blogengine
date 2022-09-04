render: {
	baseUrl: string
	dst?:    string | *""
	gtm?:    string
	src:     "src"
	style:   "compact" | *"full"
}

firebase?: {
	site: string
	redirects?: [...{
		glob:    string
		locaton: string
		code:    int
	}]
	headers?: [...{
		glob: string
		headers: [string]: string
	}]
}
