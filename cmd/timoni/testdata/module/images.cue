package main

// The container images tracked from the upstream releases.
values: {
	client: image: {
		repository: *"cgr.dev/chainguard/timoni" | string
		tag:        *"latest-dev" | string
		digest:     *"sha256:b49fbaac0eedc22c1cfcd26684707179cccbed0df205171bae3e1bae61326a10" | string
	}
}
