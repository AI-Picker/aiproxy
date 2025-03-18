package model

import (
	"time"

	"github.com/bytedance/sonic"
	"github.com/labring/aiproxy/relay/relaymode"
)

type ChannelTest struct {
	TestAt      time.Time      `json:"test_at"`
	Model       string         `gorm:"primaryKey"   json:"model"`
	ActualModel string         `json:"actual_model"`
	Response    string         `gorm:"type:text"    json:"response"`
	ChannelName string         `json:"channel_name"`
	ChannelType int            `json:"channel_type"`
	ChannelID   int            `gorm:"primaryKey"   json:"channel_id"`
	Took        float64        `json:"took"`
	Success     bool           `json:"success"`
	Mode        relaymode.Mode `json:"mode"`
	Code        int            `json:"code"`
}

func (ct *ChannelTest) MarshalJSON() ([]byte, error) {
	type Alias ChannelTest
	return sonic.Marshal(&struct {
		*Alias
		TestAt int64 `json:"test_at"`
	}{
		Alias:  (*Alias)(ct),
		TestAt: ct.TestAt.UnixMilli(),
	})
}
