package values

import "github.com/naveego/bosun/pkg/filter"

type ValueSetCollectionProvider interface {
	GetValueSetCollection() ValueSetCollection
}

func ExtractValueSet(provider ValueSetCollectionProvider, argsProvider filter.MatchMapArgContainer) ValueSet {

	vsc := provider.GetValueSetCollection()

	args := argsProvider.GetMatchMapArgs()

	result := vsc.ExtractValueSet(ExtractValueSetArgs{
		ExactMatch: args,
	})

	return result
}

func ExtractValueSetAdvanced(provider ValueSetCollectionProvider, args ExtractValueSetArgs) ValueSet {

	vsc := provider.GetValueSetCollection()

	result := vsc.ExtractValueSet(args)

	return result
}
