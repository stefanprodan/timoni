package main

// The container images tracked from the upstream releases.
values: {
	image: {
		repository: *"docker.io/redis" | string
		tag:        *"8.10.1-alpine" | string
		digest:     *"sha256:becdda6c7f4b3fb42e42fd7f120bbf5c54c4caaaf16f26da24e4563d2c1f0576" | string
	}
}
