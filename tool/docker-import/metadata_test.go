package main

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestReadRepositories(t *testing.T) {
	var testCases = []struct {
		input   []byte
		urls    []string
		comment string
	}{
		{
			comment: "single image",
			input: []byte(`
{
  "gcr.io/google_containers/nettest": {
    "1.8": "66f73e0947f3221e820b92e522331c6c6bd4119acfb6b42928755bd1cfa77f0c"
  }
}
`),
			urls: []string{"gcr.io/google_containers/nettest:1.8"},
		},
		{
			comment: "multiple images",
			input: []byte(`
{
  "gcr.io/google_containers/nettest": {
    "1.8": "66f73e0947f3221e820b92e522331c6c6bd4119acfb6b42928755bd1cfa77f0c"
  },
  "gcr.io/google_containers/pause": {
    "3.2": "a38657cdc544bd8c100ef952fc60e31709509b0bc4f31804ab107fe7fd0e6f4a"
  }
}
`),
			urls: []string{
				"gcr.io/google_containers/nettest:1.8",
				"gcr.io/google_containers/pause:3.2",
			},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.comment, func(t *testing.T) {
			var repos Repositories
			err := json.Unmarshal([]byte(tc.input), &repos)
			require.Nil(t, err)
			var urls []string
			for _, image := range repos.Images() {
				urls = append(urls, image.url())
			}
			require.Equal(t, urls, tc.urls)
		})
	}
}
