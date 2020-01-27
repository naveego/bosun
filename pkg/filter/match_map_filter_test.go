package filter_test

import (
	. "github.com/naveego/bosun/pkg/filter"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("MatchMapFilter", func() {

	It("should match on positive match", func(){
		config := MatchMapConfig{
			"k1": MatchMapConfigValues{"v1","v2"},
		}
		args := MatchMapArgs{
			"k1": "v1",
		}
		Expect(config.Matches(args)).To(BeTrue())
	})
	It("should match on unspecified config", func(){
		config := MatchMapConfig{
		}
		args := MatchMapArgs{
			"k1": "v3",
		}
		Expect(config.Matches(args)).To(BeTrue())
	})

	It("should not match on positive mismatch", func(){
		config := MatchMapConfig{
			"k1": MatchMapConfigValues{"v1","v2"},
		}
		args := MatchMapArgs{
			"k1": "v3",
		}
		Expect(config.Matches(args)).To(BeFalse())
	})
	It("should match on negative match", func(){
		config := MatchMapConfig{
			"k1": MatchMapConfigValues{"!v1"},
		}
		args := MatchMapArgs{
			"k1": "v2",
		}
		Expect(config.Matches(args)).To(BeTrue())
	})

	It("should not match on negative mismatch", func(){
		config := MatchMapConfig{
			"k1": MatchMapConfigValues{"!v1"},
		}
		args := MatchMapArgs{
			"k1": "v1",
		}
		Expect(config.Matches(args)).To(BeFalse())
	})
})
