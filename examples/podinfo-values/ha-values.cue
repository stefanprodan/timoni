values: {
	_mcpu: 100
	_mem:  128
	resources: {
		requests: {
			cpu:    "\(_mcpu)m"
			memory: "\(_mem)Mi"
		}
		limits: {
			cpu:    "\(_mcpu*2)m"
			memory: "\(_mem*2)Mi"
		}
	}

	autoscaling: {
		enabled:     true
		minReplicas: 2
		maxReplicas: 10
		cpu:         90
		memory:      "\(_mem*2-10)Mi"
	}
}
