package main

// The container images tracked from the upstream releases.
values: {
	image: {
		repository: *"docker.io/nginx" | string
		tag:        *"1.31.4-alpine" | string
		digest:     *"sha256:db35bfc6b2951e7f8a72db5db120288c127ffaeeb4a6d4b95a26fead017d5913" | string
	}
	test: image: {
		repository: *"docker.io/curlimages/curl" | string
		tag:        *"8.21.0" | string
		digest:     *"sha256:7c12af72ceb38b7432ab85e1a265cff6ae58e06f95539d539b654f2cfa64bb13" | string
	}
}
