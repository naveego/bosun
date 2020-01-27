package ioc

func MustProvide(provider Provider, target interface{}, options ...Options) {
	if err := provider.Provide(target, options...); err != nil {
		panic(err)
	}
}
