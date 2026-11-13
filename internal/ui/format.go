package ui

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/AMC-official/ffmpeg-tui/internal/ffprobe"
	"github.com/AMC-official/ffmpeg-tui/internal/filesystem"
)

// entryTag returns the short coloured type label for a browser entry.
func (m *Model) entryTag(e filesystem.Entry) string {
	if e.IsDir {
		return m.styles.Faint.Render("DIR")
	}
	switch e.Kind {
	case filesystem.KindVideo:
		return m.styles.Accent.Render("VID")
	case filesystem.KindAudio:
		return m.styles.Accent2.Render("AUD")
	case filesystem.KindImage:
		return m.styles.Blue.Render("IMG")
	case filesystem.KindSubtitle:
		return m.styles.Warn.Render("SUB")
	}
	return m.styles.Faint.Render("   ")
}

// durationClock renders a duration as H:MM:SS.
func durationClock(p *ffprobe.Result) string {
	if p == nil {
		return "—"
	}
	return filesystem.HumanDuration(p.Duration())
}

// probeSummaryLines returns the labelled metadata block for a probe.
func (m *Model) probeSummaryLines(p *ffprobe.Result) []string {
	if p == nil {
		return []string{m.styles.Faint.Render("no metadata yet")}
	}
	st := m.styles
	var lines []string
	add := func(k, v string) {
		if v == "" || v == "—" {
			return
		}
		lines = append(lines, st.Label(k, v))
	}
	add("Container", p.Format.FormatLongName)
	add("Duration", filesystem.HumanDuration(p.Duration()))
	add("Size", filesystem.HumanSize(p.Size()))
	add("Bitrate", filesystem.HumanBitrate(p.BitRate()))
	add("Streams", fmt.Sprintf("%d total", len(p.Streams)))
	if v := p.Video(); v != nil {
		add("Video", fmt.Sprintf("%s (%s)", v.CodecName, v.CodecLongName))
		add("Picture", fmt.Sprintf("%dx%d • %.3g fps • %s", v.Width, v.Height, v.FPS(), v.PixFmt))
		add("Video rate", filesystem.HumanBitrate(v.EstimatedBitrate))
		add("Colour", strings.TrimSpace(v.ColorSpace+" "+v.ColorTransfer+" • "+v.HDRLabel()))
		add("Profile", v.Profile)
	}
	if a := p.Audio(); a != nil {
		layout := a.ChannelLayout
		if layout == "" {
			layout = fmt.Sprintf("%d channels", a.Channels)
		}
		add("Audio", fmt.Sprintf("%s • %s • %s Hz", a.CodecName, layout, a.SampleRate))
		add("Audio rate", filesystem.HumanBitrate(a.EstimatedBitrate))
		add("Language", a.Language())
	}
	if subs := p.SubtitleStreams(); len(subs) > 0 {
		var langs []string
		for _, s := range subs {
			langs = append(langs, s.Language()+"/"+s.CodecName)
		}
		add("Subtitles", strings.Join(langs, ", "))
	}
	if atts := p.AttachmentStreams(); len(atts) > 0 {
		var names []string
		for _, a := range atts {
			names = append(names, firstNonEmpty(a.AttachmentName, a.CodecName))
		}
		add("Attachments", strings.Join(names, ", "))
	}
	if len(p.Chapters) > 0 {
		add("Chapters", fmt.Sprintf("%d", len(p.Chapters)))
	}
	for _, key := range []string{"title", "artist", "album", "genre", "date", "comment"} {
		if v := p.Format.Tags[key]; v != "" {
			add(strings.Title(key), v)
		}
	}
	return lines
}

// streamRows builds the stream viewer table.
func streamRows(p *ffprobe.Result) [][]string {
	if p == nil {
		return nil
	}
	var rows [][]string
	for _, s := range p.Streams {
		details := ""
		switch s.CodecType {
		case "video":
			details = fmt.Sprintf("%dx%d %.3gfps %s", s.Width, s.Height, s.FPS(), s.PixFmt)
			if s.IsHDR() {
				details += " " + s.HDRLabel()
			}
		case "audio":
			layout := s.ChannelLayout
			if layout == "" {
				layout = fmt.Sprintf("%dch", s.Channels)
			}
			details = fmt.Sprintf("%s %s %sHz", s.CodecName, layout, s.SampleRate)
		case "subtitle":
			details = s.CodecLongName
		case "attachment":
			details = firstNonEmpty(s.AttachmentName, s.CodecName)
		default:
			details = s.CodecLongName
		}
		flags := []string{}
		if s.IsDefault {
			flags = append(flags, "default")
		}
		if s.IsForced {
			flags = append(flags, "forced")
		}
		if s.Disposition["attached_pic"] == 1 {
			flags = append(flags, "cover")
		}
		rate := filesystem.HumanBitrate(s.EstimatedBitrate)
		rows = append(rows, []string{
			strconv.Itoa(s.Index),
			s.CodecType,
			s.CodecName,
			details,
			rate,
			s.Language(),
			strings.Join(flags, ","),
		})
	}
	return rows
}

// mediaExtCompletions returns container extensions for completion.
func mediaExtCompletions() []string {
	out := make([]string, 0, 16)
	for _, ext := range []string{"mp4", "mkv", "mov", "webm", "avi", "mp3", "m4a", "flac", "wav", "ogg", "gif", "png", "jpg", "webp", "avif"} {
		out = append(out, "."+ext)
	}
	return out
}

// parseBitrateString parses ffprobe's numeric bitrate strings.
func parseBitrateString(s string) int64 {
	n, _ := strconv.ParseInt(s, 10, 64)
	return n
}
