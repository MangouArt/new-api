package common

import (
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/stretchr/testify/require"
)

func TestGetFullRequestURLDoesNotDuplicateV1WhenBaseURLAlreadyEndsWithV1(t *testing.T) {
	actual := GetFullRequestURL("https://dm-fox.rjj.cc/codex/v1", "/v1/images/generations", constant.ChannelTypeOpenAI)
	require.Equal(t, "https://dm-fox.rjj.cc/codex/v1/images/generations", actual)
}
