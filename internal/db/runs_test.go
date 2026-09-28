package db

import (
	"context"
	"testing"
	"time"

	"github.com/hsk-kr/swingdesk/internal/model"
)

func TestRunLifecycle(t *testing.T) {
	conn, _ := openTemp(t)
	ctx := context.Background()
	if _, ok, err := LastFinishedRun(ctx, conn); ok || err != nil {
		t.Fatalf("empty db: ok=%v err=%v", ok, err)
	}
	a, err := StartRun(ctx, conn, t0)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := StartRun(ctx, conn, t0.Add(time.Minute))
	stale, err := StaleRuns(ctx, conn, b)
	if err != nil || len(stale) != 1 || stale[0].ID != a {
		t.Fatalf("stale = %+v, %v", stale, err)
	}
	if err := FinishRun(ctx, conn, Run{ID: a, FinishedAt: t0.Add(2 * time.Minute), Status: model.RunPartial, Error: "tech: timeout", JobsOK: 2, JobsFail: 1}); err != nil {
		t.Fatal(err)
	}
	last, ok, err := LastFinishedRun(ctx, conn)
	if err != nil || !ok || last.ID != a || last.Status != model.RunPartial || last.Error != "tech: timeout" || last.JobsOK != 2 {
		t.Errorf("last = %+v ok=%v err=%v", last, ok, err)
	}
	if err := RecordLateJob(ctx, conn, a, 3); err != nil {
		t.Fatal(err)
	}
	last, _, _ = LastFinishedRun(ctx, conn)
	if last.Status != model.RunOK || last.JobsOK != 3 || last.JobsFail != 0 {
		t.Errorf("after late job = %+v", last)
	}
	if err := RecordLateJob(ctx, conn, b, 3); err != nil {
		t.Fatal(err)
	}
	stale, _ = StaleRuns(ctx, conn, 0)
	if len(stale) != 1 || stale[0].JobsOK != 1 {
		t.Errorf("running run credited but must stay running: %+v", stale)
	}
	if err := FinishRun(ctx, conn, Run{ID: b, Status: model.RunRunning}); err == nil {
		t.Error("finishing as running should fail")
	}
	if err := FinishRun(ctx, conn, Run{ID: 999, Status: model.RunOK}); err == nil {
		t.Error("finishing a missing run should fail")
	}
	if at, ok, err := RunStartedAt(ctx, conn, a); !ok || err != nil || !at.Equal(t0) {
		t.Errorf("run a started = %v %v %v", at, ok, err)
	}
	if _, ok, _ := RunStartedAt(ctx, conn, 999); ok {
		t.Error("run 999 should not exist")
	}
	// Crediting never exceeds the job total.
	for range 3 {
		if err := RecordLateJob(ctx, conn, a, 3); err != nil {
			t.Fatal(err)
		}
	}
	last, _, _ = LastFinishedRun(ctx, conn)
	if last.JobsOK != 3 || last.JobsFail != 0 || last.Status != model.RunOK {
		t.Errorf("over-credited run = %+v", last)
	}
}
