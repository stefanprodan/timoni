@if(test)

package main

values: {
	image: digest:        "sha256:b68bfced7e4dbf9961c33bab7a36c5b80364dbd3390a496e9801411987ca296a"
	autoscaling: enabled: true
	ingress: enabled:     true
	monitoring: enabled:  true
}
