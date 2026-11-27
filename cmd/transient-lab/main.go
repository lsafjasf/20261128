package main

import (
	"encoding/csv"
	"flag"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strconv"

	"transient-bitdepth-lab/internal/analyze"
	"transient-bitdepth-lab/internal/quant"
	"transient-bitdepth-lab/internal/synth"
	"transient-bitdepth-lab/internal/wavio"
)

type method struct {
	name        string
	output      []int32
	independent []int32
	reference   []int32
}

func main() {
	inputPath := flag.String("in", "", "optional mono 24-bit WAV input; a synthetic kick/bass WAV is generated when omitted")
	outDir := flag.String("out", "out", "output directory")
	headroom := flag.Float64("headroom-db", -1.0, "headroom required before 16-bit quantization")
	seed := flag.Uint64("seed", 20261128, "deterministic dither seed")
	flag.Parse()

	if err := run(*inputPath, *outDir, *headroom, *seed); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(inputPath, outDir string, headroomDB float64, seed uint64) error {
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return err
	}
	sampleRate := 48000
	var sourcePCM wavio.PCM
	if inputPath == "" {
		sourcePCM = wavio.PCM{SampleRate: sampleRate, Samples: synth.KickBass(sampleRate, 1.2)}
		inputPath = filepath.Join(outDir, "source-24bit.wav")
		if err := wavio.WriteMono24(inputPath, sourcePCM); err != nil {
			return err
		}
	} else {
		var err error
		sourcePCM, err = wavio.ReadMono24(inputPath)
		if err != nil {
			return err
		}
		sampleRate = sourcePCM.SampleRate
	}

	source24 := quant.FloatToInt24(sourcePCM.Samples)
	corrected24, headroomGain := quant.ApplyHeadroom(source24, headroomDB)
	methods := []method{
		{
			name:        "truncate",
			output:      quant.Truncate(source24),
			independent: quant.Truncate(source24),
			reference:   source24,
		},
		{
			name:        "tpdf_dither",
			output:      quant.TPDFDither(source24, seed),
			independent: quant.TPDFDither(source24, seed+97),
			reference:   source24,
		},
		{
			name:        "shaped_tpdf",
			output:      quant.ShapedTPDF(source24, seed),
			independent: quant.ShapedTPDF(source24, seed+97),
			reference:   source24,
		},
		{
			name:        "corrected_shaped_tpdf",
			output:      quant.ShapedTPDF(corrected24, seed),
			independent: quant.ShapedTPDF(corrected24, seed+97),
			reference:   corrected24,
		},
	}

	results := make([]analyze.Result, 0, len(methods))
	for _, candidate := range methods {
		result, rows := analyze.CompareIndependent(candidate.name, sampleRate, candidate.reference, candidate.output, candidate.independent)
		results = append(results, result)
		if err := writeWAV(filepath.Join(outDir, candidate.name+"-16bit.wav"), sampleRate, candidate.output); err != nil {
			return err
		}
		if err := writeFrames(filepath.Join(outDir, candidate.name+"-frames.csv"), rows); err != nil {
			return err
		}
	}
	if err := writeSummary(filepath.Join(outDir, "summary.csv"), results); err != nil {
		return err
	}
	return writeReport(filepath.Join(outDir, "report.md"), inputPath, headroomDB, headroomGain, results)
}

func writeWAV(path string, sampleRate int, values []int32) error {
	return wavio.WriteMono16(path, wavio.PCM{
		SampleRate: sampleRate,
		Samples:    quant.Int16ToFloat(values),
	})
}

func writeSummary(path string, results []analyze.Result) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()
	header := []string{
		"method", "source_peak_dbfs", "output_peak_dbfs", "peak_delta_db",
		"transient_energy_delta_db", "attack_slope_delta_db", "residual_rms_lsb",
		"residual_peak_lsb", "residual_transient_rms_lsb", "residual_transient_peak_lsb",
		"residual_dc_lsb", "residual_correlation",
		"residual_repeat_correlation", "residual_zcr", "residual_hpf_rms_lsb",
		"clamped_samples", "onsets",
	}
	if err := writer.Write(header); err != nil {
		return err
	}
	for _, result := range results {
		row := []string{
			result.Name,
			fmt.Sprintf("%.3f", toDBFS(result.PeakSource)),
			fmt.Sprintf("%.3f", toDBFS(result.PeakOutput)),
			fmt.Sprintf("%.4f", result.PeakDeltaDB),
			fmt.Sprintf("%.4f", result.TransientEnergyRatio),
			fmt.Sprintf("%.4f", result.AttackSlopeRatio),
			fmt.Sprintf("%.4f", result.ResidualRMS*math.MaxInt16),
			fmt.Sprintf("%.4f", result.ResidualPeak*math.MaxInt16),
			fmt.Sprintf("%.4f", result.ResidualTransientRMS*math.MaxInt16),
			fmt.Sprintf("%.4f", result.ResidualTransientPeak*math.MaxInt16),
			fmt.Sprintf("%.5f", result.ResidualDC*math.MaxInt16),
			fmt.Sprintf("%.5f", result.ResidualCorrelation),
			fmt.Sprintf("%.5f", result.ResidualRepeatCorr),
			fmt.Sprintf("%.5f", result.ResidualZCR),
			fmt.Sprintf("%.4f", result.ResidualHPF*math.MaxInt16),
			strconv.Itoa(result.ClampedSamples),
			strconv.Itoa(len(result.Onsets)),
		}
		if err := writer.Write(row); err != nil {
			return err
		}
	}
	return writer.Error()
}

func writeFrames(path string, rows []analyze.FrameRow) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()
	if err := writer.Write([]string{"frame", "start_sample", "source_rms_lsb", "output_rms_lsb", "residual_env_lsb", "residual_hpf_rms_lsb", "transient_window"}); err != nil {
		return err
	}
	for _, row := range rows {
		values := []string{
			strconv.Itoa(row.Frame),
			strconv.Itoa(row.StartSample),
			fmt.Sprintf("%.5f", row.SourceEnergy),
			fmt.Sprintf("%.5f", row.OutputEnergy),
			fmt.Sprintf("%.5f", row.ResidualEnvelope),
			fmt.Sprintf("%.5f", row.ResidualHPF),
			strconv.FormatBool(row.TransientWindow),
		}
		if err := writer.Write(values); err != nil {
			return err
		}
	}
	return writer.Error()
}

func writeReport(path, inputPath string, headroomDB float64, headroomGain float64, results []analyze.Result) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()

	if _, err := fmt.Fprintf(file, "# 24-bit to 16-bit Transient Audit\n\n"); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(file, "- Input: `%s`\n- Headroom target: %.2f dBFS\n- Applied headroom gain: %.6f (%.3f dB)\n\n", inputPath, headroomDB, headroomGain, toDB(headroomGain)); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(file, "| method | src peak dBFS | out peak dBFS | peak Δ dB | transient E Δ dB | attack Δ dB | residual RMS LSB | residual peak LSB | transient residual RMS LSB | transient residual peak LSB | corr | repeat corr | HPF residual LSB | clips |"); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(file, "|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|"); err != nil {
		return err
	}
	for _, result := range results {
		_, err := fmt.Fprintf(file, "| %s | %.3f | %.3f | %.4f | %.4f | %.4f | %.4f | %.4f | %.4f | %.4f | %.4f | %.4f | %.4f | %d |\n",
			result.Name,
			toDBFS(result.PeakSource),
			toDBFS(result.PeakOutput),
			result.PeakDeltaDB,
			result.TransientEnergyRatio,
			result.AttackSlopeRatio,
			result.ResidualRMS*math.MaxInt16,
			result.ResidualPeak*math.MaxInt16,
			result.ResidualTransientRMS*math.MaxInt16,
			result.ResidualTransientPeak*math.MaxInt16,
			result.ResidualCorrelation,
			result.ResidualRepeatCorr,
			result.ResidualHPF*math.MaxInt16,
			result.ClampedSamples,
		)
		if err != nil {
			return err
		}
	}
	_, err = fmt.Fprint(file, `
Interpretation:
- Truncation has repeat correlation of 1: the same input always produces the same error, even when its instantaneous source correlation is low.
- TPDF dither decorrelates independent quantization runs; its residual RMS is close to the ideal 1/sqrt(12) LSB, while transient energy and peak remain statistically unbiased.
- Shaped TPDF moves residual energy above the low-frequency band; inspect HPF residual together with residual peak because noise shaping can create short overshoots.
- The corrected path reserves headroom before shaped TPDF quantization, preventing full-scale clamps without adding a limiter or altering transient timing.
- Per-frame CSV files show residual envelopes inside and outside transient windows; do not judge the result using a static spectrum alone.
`)
	return err
}

func toDBFS(amplitude float64) float64 {
	if amplitude <= 0 {
		return math.Inf(-1)
	}
	return 20 * math.Log10(amplitude)
}

func toDB(ratio float64) float64 {
	if ratio <= 0 {
		return math.Inf(-1)
	}
	return 20 * math.Log10(ratio)
}
