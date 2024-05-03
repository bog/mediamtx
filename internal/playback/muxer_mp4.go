package playback

import (
	"io"

	"github.com/bluenviron/mediacommon/pkg/formats/fmp4"
	"github.com/bluenviron/mediamtx/internal/playback/mp4"
)

type muxerMP4Track struct {
	mp4.Track
	lastDTS int64
}

func findTrackMP4(tracks []*muxerMP4Track, id int) *muxerMP4Track {
	for _, track := range tracks {
		if track.ID == id {
			return track
		}
	}
	return nil
}

type muxerMP4 struct {
	w io.Writer

	tracks   []*muxerMP4Track
	curTrack *muxerMP4Track
}

func (w *muxerMP4) writeInit(init *fmp4.Init) {
	w.tracks = make([]*muxerMP4Track, len(init.Tracks))

	for i, track := range init.Tracks {
		w.tracks[i] = &muxerMP4Track{
			Track: mp4.Track{
				ID:        track.ID,
				TimeScale: track.TimeScale,
				Codec:     track.Codec,
			},
		}
	}
}

func (w *muxerMP4) setTrack(trackID int) {
	w.curTrack = findTrackMP4(w.tracks, trackID)
}

func (w *muxerMP4) writeSample(
	dts int64,
	sliceDts int64,
	ptsOffset int32,
	isNonSyncSample bool,
	payloadSize uint32,
	getPayload func() ([]byte, error),
) error {
	// remove GOPs before the GOP of the requested slice
	if (dts < sliceDts || (dts >= sliceDts && w.curTrack.lastDTS < sliceDts)) && !isNonSyncSample {
		w.curTrack.Samples = nil
	}

	if w.curTrack.Samples == nil {
		w.curTrack.TimeOffset = int32(dts)
	} else {
		diff := dts - w.curTrack.lastDTS
		if diff < 0 {
			diff = 0
		}
		w.curTrack.Samples[len(w.curTrack.Samples)-1].Duration = uint32(diff)
	}

	// prevent warning "edit list: 1 Missing key frame while searching for timestamp: 0"
	if !isNonSyncSample {
		ptsOffset = 0
	}

	w.curTrack.Samples = append(w.curTrack.Samples, &mp4.Sample{
		PTSOffset:       ptsOffset,
		IsNonSyncSample: isNonSyncSample,
		PayloadSize:     payloadSize,
		GetPayload:      getPayload,
	})
	w.curTrack.lastDTS = dts

	return nil
}

func (w *muxerMP4) writeFinalDTS(dts int64) {
	diff := dts - w.curTrack.lastDTS
	if diff < 0 {
		diff = 0
	}
	w.curTrack.Samples[len(w.curTrack.Samples)-1].Duration = uint32(diff)
}

func (w *muxerMP4) flush() error {
	h := mp4.Presentation{
		Tracks: make([]*mp4.Track, len(w.tracks)),
	}

	for i, track := range w.tracks {
		h.Tracks[i] = &track.Track
	}

	return h.Marshal(w.w)
}
