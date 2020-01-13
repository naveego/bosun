package values

type ValueSetChain struct {
	ValueSets ValueSets
}

func (v *ValueSetChain) Append(source string, valueSet ValueSet) {
	valueSet.Source = source
	v.ValueSets = append(v.ValueSets, valueSet)
}

func (v *ValueSetChain) Prepend(source string, valueSet ValueSet) {
	valueSet.Source = source
	v.ValueSets = append(ValueSets{valueSet}, valueSet)
}
