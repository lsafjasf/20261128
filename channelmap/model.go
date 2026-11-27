package channelmap

// Channel describes one channel in encoded order. Index is zero-based;
// Position is one-based and is the value shown to operators.
type Channel struct {
	Index    int    `json:"index"`
	Position int    `json:"position"`
	Label    Label  `json:"label"`
	Name     string `json:"name"`
}

// Layout is the channel order read from an audio container's encoding
// metadata. It is never inferred from the delivery channel map.
type Layout struct {
	Container   string    `json:"container"`
	Codec       string    `json:"codec,omitempty"`
	SampleRate  int       `json:"sample_rate,omitempty"`
	ChannelMask *uint32   `json:"channel_mask,omitempty"`
	Channels    []Channel `json:"channels"`
	Source      string    `json:"source,omitempty"`
}

// DeliveryChannel is one target slot in the external delivery channel map.
// SourceRef is optional; when present it binds the slot explicitly to a
// container channel by canonical label or zero-based source index.
type DeliveryChannel struct {
	Position  int    `json:"position"`
	Label     Label  `json:"label"`
	Name      string `json:"name"`
	SourceRef string `json:"source_ref,omitempty"`
}

// DeliveryMap is the externally supplied order expected by monitoring or
// broadcast. Its slots are one-based.
type DeliveryMap struct {
	Name     string            `json:"name,omitempty"`
	Source   string            `json:"source,omitempty"`
	Channels []DeliveryChannel `json:"channels"`
}

// ChannelMapping is one row of the generated source-to-delivery permutation.
type ChannelMapping struct {
	Source    Channel         `json:"source"`
	Delivery  DeliveryChannel `json:"delivery"`
	Reordered bool            `json:"reordered"`
}

// Plan is the complete, validated per-channel mapping.
type Plan struct {
	SchemaVersion int              `json:"schema_version"`
	Container     Layout           `json:"container"`
	Delivery      DeliveryMap      `json:"delivery"`
	Mappings      []ChannelMapping `json:"mappings"`
	Identity      bool             `json:"identity"`
}

func makeChannels(labels []Label) []Channel {
	channels := make([]Channel, len(labels))
	for i, label := range labels {
		channels[i] = Channel{
			Index:    i,
			Position: i + 1,
			Label:    label,
			Name:     label.Name(),
		}
	}
	return channels
}
