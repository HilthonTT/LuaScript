package training

import (
	"fmt"
	"os"
	"text/tabwriter"
	"time"

	"github.com/hilthontt/luascript/internal/native"
	"github.com/hilthontt/luascript/internal/native/datascience/ml"
)

type StatsPrinter struct {
	w *tabwriter.Writer
}

func NewStatsPrinter() *StatsPrinter {
	return &StatsPrinter{
		w: tabwriter.NewWriter(os.Stdout, 16, 0, 3, ' ', 0),
	}
}

func (p *StatsPrinter) Init(n *ml.Neural) {
	fmt.Fprintf(p.w, "Epochs\tElapsed\tLoss (%s)\t", n.Config.Loss)
	if n.Config.Mode == ml.ModeMultiClass {
		fmt.Fprintf(p.w, "Accuracy\t\n---\t---\t---\t---\t\n")
	} else {
		fmt.Fprintf(p.w, "\n---\t---\t---\t\n")
	}
}

func (p *StatsPrinter) PrintProgress(n *ml.Neural, validation Examples, elapsed time.Duration, iteration int) {
	fmt.Fprintf(p.w, "%d\t%s\t%.4f\t%s\n",
		iteration,
		elapsed.String(),
		crossValidate(n, validation),
		formatAccuracy(n, validation))
	p.w.Flush()
}

func formatAccuracy(n *ml.Neural, validation Examples) string {
	if n.Config.Mode == ml.ModeMultiClass {
		return fmt.Sprintf("%.2f\t", accuracy(n, validation))
	}
	return ""
}

func accuracy(n *ml.Neural, validation Examples) float64 {
	correct := 0
	for _, e := range validation {
		est := n.Predict(e.Input)
		if native.ArgMax(e.Response) == native.ArgMax(est) {
			correct++
		}
	}
	return float64(correct) / float64(len(validation))
}

func crossValidate(n *ml.Neural, validation Examples) float64 {
	predictions, responses := make([][]float64, len(validation)), make([][]float64, len(validation))
	for i := 0; i < len(validation); i++ {
		predictions[i] = n.Predict(validation[i].Input)
		responses[i] = validation[i].Response
	}

	return ml.GetLoss(n.Config.Loss).F(predictions, responses)
}
