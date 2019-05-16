package util

import (
	"reflect"
	"testing"
)

func TestStringSliceToMap(t *testing.T) {
	type args struct {
		ss []string
	}
	tests := []struct {
		name string
		args args
		want map[string]string
	}{
		{
			name: "combine",
			args: args{ss: []string{"a", "A", "b", "B"}},
			want: map[string]string{
				"a": "A",
				"b": "B",
			},
		}, {
			name: "combine with extra",
			args: args{ss: []string{"a", "A", "b", "B", "c"}},
			want: map[string]string{
				"a": "A",
				"b": "B",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := StringSliceToMap(tt.args.ss...); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("StringSliceToMap() = %v, want %v", got, tt.want)
			}
		})
	}
}
