package loader

type lookupEntry interface {
	Name() string
}

func lookupByLabel(source func() ([]lookupEntry, error), label string) (lookupEntry, error) {
	candidates, err := source()
	if err != nil {
		return nil, err
	}

	byLabel := make(map[string]lookupEntry, len(candidates))
	for _, candidate := range candidates {
		byLabel[candidate.Name()] = candidate
	}

	return byLabel[label], nil
}
