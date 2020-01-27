package bosun

import (
	"github.com/naveego/bosun/pkg/values"
	"github.com/pkg/errors"
)

func ResolveValues(collectionProvider values.ValueSetCollectionProvider, ctx BosunContext) (values.ValueSet, error) {

	extracted := values.ExtractValueSet(collectionProvider, ctx)

	loaded, err := extracted.WithFilesLoaded(ctx)
	if err != nil {
		return loaded, errors.Wrapf(err, "load value set files")
	}

	return loaded, nil

}
