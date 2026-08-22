package main

// The container images tracked from the upstream releases.
values: {
	image: {
		repository: *"docker.io/nginxinc/nginx-unprivileged" | string
		tag:        *"1-alpine" | string
		digest:     *"" | string
	}
}
