package test

// these secret values are injected at apply time from OS ENV
secrets: {
	username: *"test" | string @timoni(env:string:USERNAME)

	// The OpenPGP key will be injected as a multi-line string
	key: string @timoni(env:string:PGP_PUB_KEY)

	age:     int  @timoni(env:number:AGE)
	isAdmin: bool @timoni(env:bool:IS_ADMIN)
}
