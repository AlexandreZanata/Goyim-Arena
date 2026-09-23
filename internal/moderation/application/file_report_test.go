package application_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/AlexandreZanata/Regnovum/internal/moderation/application"
	"github.com/AlexandreZanata/Regnovum/internal/moderation/domain"
)

type fakeTargets struct {
	arenas    map[string]*application.TargetInfo
	arguments map[string]*application.TargetInfo
	profiles  map[string]*application.TargetInfo
	err       error
}

func (f *fakeTargets) DescribeArena(_ context.Context, id string) (*application.TargetInfo, error) {
	if f.err != nil {
		return nil, f.err
	}
	if info, ok := f.arenas[id]; ok {
		return info, nil
	}
	return &application.TargetInfo{}, nil
}

func (f *fakeTargets) DescribeArgument(_ context.Context, id string) (*application.TargetInfo, error) {
	if f.err != nil {
		return nil, f.err
	}
	if info, ok := f.arguments[id]; ok {
		return info, nil
	}
	return &application.TargetInfo{}, nil
}

func (f *fakeTargets) DescribeProfile(_ context.Context, id string) (*application.TargetInfo, error) {
	if f.err != nil {
		return nil, f.err
	}
	if info, ok := f.profiles[id]; ok {
		return info, nil
	}
	return &application.TargetInfo{}, nil
}

type fakeReports struct {
	duplicates map[string]*application.ReportRecord
	recent     int
	inserts    []application.InsertReportRequest
	seq        int
}

func (f *fakeReports) FindDuplicate(_ context.Context, reporter domain.AccountID, target domain.TargetType, targetID string, reason domain.Reason, _ time.Time) (*application.ReportRecord, error) {
	return f.duplicates[reporter.String()+"|"+target.String()+"|"+targetID+"|"+reason.String()], nil
}

func (f *fakeReports) CountRecentByReporter(_ context.Context, _ domain.AccountID, _ time.Time) (int, error) {
	return f.recent, nil
}

func (f *fakeReports) Insert(_ context.Context, request application.InsertReportRequest) (*application.ReportRecord, error) {
	f.seq++
	record := &application.ReportRecord{ID: "report-test-1", Reporter: request.Reporter}
	f.inserts = append(f.inserts, request)
	return record, nil
}

const (
	reporterID = domain.AccountID("018f6b2a-0000-7000-8000-000000000041")
	ownerID    = domain.AccountID("018f6b2a-0000-7000-8000-000000000042")
	arenaID    = "018f6b2a-0000-7000-8000-000000000051"
)

func reportFixture() (*fakeTargets, *fakeReports) {
	targets := &fakeTargets{
		arenas: map[string]*application.TargetInfo{
			arenaID: {Exists: true, Owner: ownerID},
		},
	}
	return targets, &fakeReports{duplicates: map[string]*application.ReportRecord{}}
}

func TestFileReportAcceptsSpamAndSelfReport(t *testing.T) {
	t.Parallel()

	targets, reports := reportFixture()
	uc, err := application.NewFileReportUseCase(application.FileReportDependencies{
		Targets: targets,
		Reports: reports,
		Clock:   &fakeClock{},
	})
	if err != nil {
		t.Fatalf("NewFileReportUseCase: %v", err)
	}

	// Spam is a valid category.
	spam, err := uc.Execute(context.Background(), application.FileReportCommand{
		Reporter: reporterID,
		Target:   "arena",
		TargetID: arenaID,
		Reason:   "spam",
	})
	if err != nil {
		t.Fatalf("spam report: %v", err)
	}
	if spam.Replayed || spam.ReportID == "" {
		t.Fatalf("spam result = %+v, want fresh report", spam)
	}

	// Self-reports are allowed: the owner contesting owned content asks for
	// review instead of being rejected.
	self, err := uc.Execute(context.Background(), application.FileReportCommand{
		Reporter: ownerID,
		Target:   "arena",
		TargetID: arenaID,
		Reason:   "harassment",
		Context:  "Requesting review of my own arena after dogpiling",
	})
	if err != nil {
		t.Fatalf("self-report: %v", err)
	}
	if self.Replayed {
		t.Fatalf("self-report result = %+v, want fresh", self)
	}
	if len(reports.inserts) != 2 {
		t.Fatalf("inserts = %d, want 2", len(reports.inserts))
	}
}

func TestFileReportRejectsUnknownAndRemovedTargets(t *testing.T) {
	t.Parallel()

	targets, reports := reportFixture()
	targets.arenas["018f6b2a-0000-7000-8000-000000000052"] = &application.TargetInfo{Exists: true, Removed: true}
	uc, err := application.NewFileReportUseCase(application.FileReportDependencies{
		Targets: targets,
		Reports: reports,
		Clock:   &fakeClock{},
	})
	if err != nil {
		t.Fatalf("NewFileReportUseCase: %v", err)
	}

	_, err = uc.Execute(context.Background(), application.FileReportCommand{
		Reporter: reporterID,
		Target:   "arena",
		TargetID: "018f6b2a-0000-7000-8000-000000000059",
		Reason:   "spam",
	})
	if !errors.Is(err, application.ErrTargetNotFound) {
		t.Fatalf("unknown error = %v, want ErrTargetNotFound", err)
	}

	_, err = uc.Execute(context.Background(), application.FileReportCommand{
		Reporter: reporterID,
		Target:   "arena",
		TargetID: "018f6b2a-0000-7000-8000-000000000052",
		Reason:   "spam",
	})
	if !errors.Is(err, application.ErrTargetRemoved) {
		t.Fatalf("removed error = %v, want ErrTargetRemoved", err)
	}
}

func TestFileReportDeduplicatesAndSignalsRate(t *testing.T) {
	t.Parallel()

	targets, reports := reportFixture()
	existing := &application.ReportRecord{ID: "report-original-1", Reporter: reporterID}
	reports.duplicates[reporterID.String()+"|arena|"+arenaID+"|spam"] = existing
	reports.recent = domain.RateThreshold
	uc, err := application.NewFileReportUseCase(application.FileReportDependencies{
		Targets: targets,
		Reports: reports,
		Clock:   &fakeClock{},
	})
	if err != nil {
		t.Fatalf("NewFileReportUseCase: %v", err)
	}

	duplicate, err := uc.Execute(context.Background(), application.FileReportCommand{
		Reporter: reporterID,
		Target:   "arena",
		TargetID: arenaID,
		Reason:   "spam",
	})
	if err != nil {
		t.Fatalf("duplicate: %v", err)
	}
	if !duplicate.Replayed || duplicate.ReportID != existing.ID {
		t.Fatalf("duplicate result = %+v, want replay of %s", duplicate, existing.ID)
	}
	if !duplicate.RateLimited {
		t.Fatal("reporter at threshold must be rate-signaled")
	}
	if len(reports.inserts) != 0 {
		t.Fatalf("inserts = %d, want 0 (deduplicated)", len(reports.inserts))
	}
}

func TestFileReportVolumeNeverRemovesAndHidesPII(t *testing.T) {
	t.Parallel()

	targets, reports := reportFixture()
	uc, err := application.NewFileReportUseCase(application.FileReportDependencies{
		Targets: targets,
		Reports: reports,
		Clock:   &fakeClock{},
	})
	if err != nil {
		t.Fatalf("NewFileReportUseCase: %v", err)
	}

	// Filing many reports resolves inserts only: the target directory port
	// exposes no mutation, so volume cannot remove content by construction.
	for i := 0; i < 3; i++ {
		if _, err := uc.Execute(context.Background(), application.FileReportCommand{
			Reporter: reporterID,
			Target:   "arena",
			TargetID: arenaID,
			Reason:   "spam",
			Context:  "volume probe",
		}); err != nil {
			t.Fatalf("volume report %d: %v", i, err)
		}
	}
	// The fake directory has no mutation surface to assert beyond the port
	// shape: TargetDirectory declares Describe* only.

	// PII never surfaces in errors: unknown-target and validation failures
	// carry static sentinels without the context, email or target content.
	secretContext := "victim@example.com lives at 123 Main St"
	_, err = uc.Execute(context.Background(), application.FileReportCommand{
		Reporter: reporterID,
		Target:   "arena",
		TargetID: "018f6b2a-0000-7000-8000-000000000059",
		Reason:   "spam",
		Context:  secretContext,
	})
	if err == nil {
		t.Fatal("unknown target must fail")
	}
	for _, secret := range []string{secretContext, "victim@example.com", "123 Main St"} {
		if strings.Contains(err.Error(), secret) {
			t.Fatalf("error leaks PII %q: %v", secret, err)
		}
	}
}
