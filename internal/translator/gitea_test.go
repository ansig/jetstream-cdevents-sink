package translator

import (
	"fmt"
	"testing"

	cdevents "github.com/cdevents/sdk-go/pkg/api"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGiteaPush(t *testing.T) {

	pushCommitPayload := `{
		"ref": "refs/heads/main",
		"before": "a359287123178c5d05654864e80ab6f3bfc3d78a",
		"after": "9d7b2d18bf7f315c666a4b3607f47bd452e7c8d2",
		"total_commits": 1,
		"repository": {
			"full_name": "yoloco/project1",
			"html_url": "http://git.example.com/yoloco/project1"
		}
	}	
	`

	pushNewBranchPayload := `{
		"ref": "refs/heads/foo",
		"before": "0000000000000000000000000000000000000000",
		"after": "9d7b2d18bf7f315c666a4b3607f47bd452e7c8d2",
		"total_commits": 0,
		"repository": {
			"full_name": "yoloco/project1",
			"html_url": "http://git.example.com/yoloco/project1"
		}
	}`

	repoWithNoHtmlUrlPayload := `{
		"after": "9d7b2d18bf7f315c666a4b3607f47bd452e7c8d2",
		"total_commits": 1,
		"repository": {
			"full_name": "yoloco/project1"
		}
	}`

	repoWithNoFullNamePayload := `{
		"after": "9d7b2d18bf7f315c666a4b3607f47bd452e7c8d2",
		"total_commits": 1,
		"repository": {
			"html_url": "http://git.example.com/yoloco/project1"
		}
	}`

	noAfterFieldPayload := `{
		"total_commits": 1,
		"repository": {
			"full_name": "yoloco/project1",
			"html_url": "http://git.example.com/yoloco/project1"
		}
	}`

	noRepoPayload := `{
		"ref": "refs/heads/main",
		"before": "a359287123178c5d05654864e80ab6f3bfc3d78a",
		"after": "9d7b2d18bf7f315c666a4b3607f47bd452e7c8d2",
		"total_commits": 1
	}	
	`

	for _, tc := range []struct {
		title             string
		payload           string
		expectedEventType interface{}
		expectedError     error
	}{
		{
			title:             "returns ChangeMergedEvent on push to main branch payload",
			payload:           pushCommitPayload,
			expectedEventType: cdevents.ChangeMergedEventTypeV0_2_0,
		},
		{
			title:         "error on push to new branch with no new commits",
			payload:       pushNewBranchPayload,
			expectedError: ErrNoCommitsOnPushEvent,
		},
		{
			title:         "error when payload missing repository HTML url field",
			payload:       repoWithNoHtmlUrlPayload,
			expectedError: ErrMissingRequiredFields,
		},
		{
			title:         "error when payload missing repository full name field",
			payload:       repoWithNoFullNamePayload,
			expectedError: ErrMissingRequiredFields,
		},
		{
			title:         "error when payload missing after field",
			payload:       noAfterFieldPayload,
			expectedError: ErrMissingRequiredFields,
		},
		{
			title:         "error when payload missing repository field",
			payload:       noRepoPayload,
			expectedError: ErrMissingRequiredFields,
		},
	} {
		t.Run(tc.title, func(t *testing.T) {
			translator := &GiteaPush{}

			cdEvent, err := translator.Translate([]byte(tc.payload))

			if tc.expectedError != nil {
				assert.Equal(t, tc.expectedError, err)
			} else {
				require.NoError(t, err, "no error should be returned when translating event")
			}

			if tc.expectedEventType != nil {
				require.NotNil(t, cdEvent, "CD event must not be nil")

				assert.Equal(t, tc.expectedEventType, cdEvent.GetType(), "Event did not have expected type")
				assert.Equal(t, "9d7b2d18bf7f315c666a4b3607f47bd452e7c8d2", cdEvent.GetSubjectId(), "Subject ID must be commit sha")
				assert.Equal(t, "git.example.com", cdEvent.GetSource(), "Event Source must be server host name")
				assert.Equal(t, "git.example.com/yoloco/project1", cdEvent.GetSubjectSource(), "Event Subject Source must be URL to project")

				subjectContent := cdEvent.GetSubjectContent()
				switch s := subjectContent.(type) {
				case cdevents.ChangeMergedSubjectContentV0_2_0:
					require.NotNil(t, s.Repository, "Content repository must not be nil")
					assert.Equal(t, "yoloco/project1", s.Repository.Id, "Content repository Id should be project full name")
				default:
					require.Fail(t, fmt.Sprintf("unexpected subject content type: %T", s))
				}
			}
		})
	}
}

func TestGiteaPullRequest(t *testing.T) {

	prOpenedPayload := `{
		"action": "opened",
		"pull_request": {
			"id": 3,
			"title": "Fix something PR"
		},
		"repository": {
			"full_name": "yoloco/project1",
			"html_url": "http://git.example.com/yoloco/project1"
		}
	}`

	prClosedPayload := `{
		"action": "closed",
		"pull_request": {
			"id": 3,
			"title": "Fix something PR"
		},
		"repository": {
			"full_name": "yoloco/project1",
			"html_url": "http://git.example.com/yoloco/project1"
		}
	}
	`

	prWithUnsupportedAction := `{
		"action": "unknown",
		"pull_request": {
			"id": 3,
			"title": "Fix something PR"
		},
		"repository": {
			"full_name": "yoloco/project1",
			"html_url": "http://git.example.com/yoloco/project1"
		}
	}`

	noActionPayload := `{
		"pull_request": {
			"id": 3,
			"title": "Fix something PR"
		},
		"repository": {
			"full_name": "yoloco/project1",
			"html_url": "http://git.example.com/yoloco/project1"
		}
	}
	`

	prWithNoIdPayload := `{
		"action": "closed",
		"pull_request": {
			"title": "Fix something PR"
		},
		"repository": {
			"full_name": "yoloco/project1",
			"html_url": "http://git.example.com/yoloco/project1"
		}
	}
	`

	prWithNoTitlePayload := `{
		"action": "opened",
		"pull_request": {
			"id": 3
		},
		"repository": {
			"full_name": "yoloco/project1",
			"html_url": "http://git.example.com/yoloco/project1"
		}
	}`

	translator := &GiteaPullRequest{}

	for _, tc := range []struct {
		title               string
		payload             string
		expectedCDEventType *cdevents.CDEventType
		expectedError       error
	}{
		{
			title:               "Return change created event on PR opened payload",
			payload:             prOpenedPayload,
			expectedCDEventType: &cdevents.ChangeCreatedEventTypeV0_3_0,
		},
		{
			title:               "Return change merged event on PR closed payload",
			payload:             prClosedPayload,
			expectedCDEventType: &cdevents.ChangeMergedEventTypeV0_2_0,
		},
		{
			title:         "Error on unsupported action",
			payload:       prWithUnsupportedAction,
			expectedError: ErrUnsupportedPRAction,
		},
		{
			title:         "Error with no action field in payload",
			payload:       noActionPayload,
			expectedError: ErrMissingRequiredFields,
		},
		{
			title:         "Error with no pull_request id field in payload",
			payload:       prWithNoIdPayload,
			expectedError: ErrMissingRequiredFields,
		},
		{
			title:         "Error with no pull_request title field in payload",
			payload:       prWithNoTitlePayload,
			expectedError: ErrMissingRequiredFields,
		},
	} {
		t.Run(tc.title, func(t *testing.T) {
			cdEvent, err := translator.Translate([]byte(tc.payload))

			if tc.expectedError != nil {
				assert.Equal(t, tc.expectedError, err)
			} else {
				require.NoError(t, err, "no error should be returned when translating event")
			}

			if tc.expectedCDEventType != nil {
				require.NotNil(t, cdEvent, "CD event must not be nil")
				assert.Equal(t, *tc.expectedCDEventType, cdEvent.GetType(), "Event must be of type ChangeCreatedEvent")
				assert.Equal(t, "git.example.com", cdEvent.GetSource(), "Event Source must be server host name")
				assert.Equal(t, "git.example.com/yoloco/project1", cdEvent.GetSubjectSource(), "Event Subject Source must be URL to project")
				assert.Equal(t, "pr-3", cdEvent.GetSubjectId(), "Subject Id must be pr-<number>")

				subjectContent := cdEvent.GetSubjectContent()
				switch s := subjectContent.(type) {
				case cdevents.ChangeCreatedSubjectContentV0_3_0:
					require.NotNil(t, s.Repository, "Content repository must not be nil")
					assert.Equal(t, "yoloco/project1", s.Repository.Id, "Content repository Id should be project full name")
					assert.Equal(t, "Fix something PR", s.Description, "Description must be PR title")
				case cdevents.ChangeMergedSubjectContentV0_2_0:
					require.NotNil(t, s.Repository, "Content repository must not be nil")
					assert.Equal(t, "yoloco/project1", s.Repository.Id, "Content repository Id should be project full name")
				default:
					require.Fail(t, fmt.Sprintf("unexpected subject content type: %T", s))
				}
			}
		})
	}
}

func TestGiteaCreate(t *testing.T) {
	branchCreatedPayload := `{
		"ref": "foo",
		"ref_type": "branch",
		"repository": {
			"full_name": "yoloco/project1",
			"html_url": "http://git.example.com/yoloco/project1"
		}
  	}`

	noRefFieldPayload := `{
		"ref_type": "branch",
		"repository": {
			"full_name": "yoloco/project1",
			"html_url": "http://git.example.com/yoloco/project1"
		}
  	}`

	unsupportedRefTypePayload := `{
		"ref": "foo",
		"ref_type": "unknown",
		"repository": {
			"full_name": "yoloco/project1",
			"html_url": "http://git.example.com/yoloco/project1"
		}
  	}`

	translator := &GiteaCreate{}

	for _, tc := range []struct {
		title               string
		payload             string
		expectedCDEventType *cdevents.CDEventType
		expectedError       error
	}{
		{
			title:               "Returns BranchCreatedEvent",
			payload:             branchCreatedPayload,
			expectedCDEventType: &cdevents.BranchCreatedEventTypeV0_2_0,
		},
		{
			title:         "Error when payload is missing ref field",
			payload:       noRefFieldPayload,
			expectedError: ErrMissingRequiredFields,
		},
		{
			title:         "Error with unsupported ref type",
			payload:       unsupportedRefTypePayload,
			expectedError: ErrUnsupportedRefType,
		},
	} {
		t.Run(tc.title, func(t *testing.T) {
			cdEvent, err := translator.Translate([]byte(tc.payload))

			if tc.expectedError != nil {
				assert.Equal(t, tc.expectedError, err)
			} else {
				require.NoError(t, err, "no error should be returned when translating event")
			}

			if tc.expectedCDEventType != nil {
				require.NotNil(t, cdEvent, "CD event must not be nil")
				assert.Equal(t, *tc.expectedCDEventType, cdEvent.GetType(), "Event must have expected type")
				assert.Equal(t, "foo", cdEvent.GetSubjectId(), "Subject ID must be name of ref")
				assert.Equal(t, "git.example.com", cdEvent.GetSource(), "Event Source must be server host name")
				assert.Equal(t, "git.example.com/yoloco/project1", cdEvent.GetSubjectSource(), "Event Subject Source must be URL to project")

				if content, ok := cdEvent.GetSubjectContent().(cdevents.BranchCreatedSubjectContentV0_2_0); ok {
					require.NotNil(t, content.Repository, "Content repository must not be nil")
					assert.Equal(t, "yoloco/project1", content.Repository.Id, "Content repository Id should be project full name")
				} else {
					require.Fail(t, "failed to cast Subject Content")
				}
			}
		})
	}
}

func TestGiteaDelete(t *testing.T) {
	branchDeletedPayload := `{
		"ref": "foo",
		"ref_type": "branch",
		"repository": {
			"full_name": "yoloco/project1",
			"html_url": "http://git.example.com/yoloco/project1"
		}
  	}`

	noRefFieldPayload := `{
		"ref_type": "branch",
		"repository": {
			"full_name": "yoloco/project1",
			"html_url": "http://git.example.com/yoloco/project1"
		}
  	}`

	unsupportedRefTypePayload := `{
		"ref": "foo",
		"ref_type": "unknown",
		"repository": {
			"full_name": "yoloco/project1",
			"html_url": "http://git.example.com/yoloco/project1"
		}
  	}`

	translator := &GiteaDelete{}

	for _, tc := range []struct {
		title               string
		payload             string
		expectedCDEventType *cdevents.CDEventType
		expectedError       error
	}{
		{
			title:               "Returns BranchDeleteEvent",
			payload:             branchDeletedPayload,
			expectedCDEventType: &cdevents.BranchDeletedEventTypeV0_2_0,
		},
		{
			title:         "Error when payload is missing ref field",
			payload:       noRefFieldPayload,
			expectedError: ErrMissingRequiredFields,
		},
		{
			title:         "Error with unsupported ref type",
			payload:       unsupportedRefTypePayload,
			expectedError: ErrUnsupportedRefType,
		},
	} {
		t.Run(tc.title, func(t *testing.T) {
			cdEvent, err := translator.Translate([]byte(tc.payload))

			if tc.expectedError != nil {
				assert.Equal(t, tc.expectedError, err)
			} else {
				require.NoError(t, err, "no error should be returned when translating event")
			}

			if tc.expectedCDEventType != nil {
				require.NotNil(t, cdEvent, "CD event must not be nil")
				assert.Equal(t, *tc.expectedCDEventType, cdEvent.GetType(), "Event must have expected type")
				assert.Equal(t, "foo", cdEvent.GetSubjectId(), "Subject ID must be name of ref")
				assert.Equal(t, "git.example.com", cdEvent.GetSource(), "Event Source must be server host name")
				assert.Equal(t, "git.example.com/yoloco/project1", cdEvent.GetSubjectSource(), "Event Subject Source must be URL to project")

				if content, ok := cdEvent.GetSubjectContent().(cdevents.BranchDeletedSubjectContentV0_2_0); ok {
					require.NotNil(t, content.Repository, "Content repository must not be nil")
					assert.Equal(t, "yoloco/project1", content.Repository.Id, "Content repository Id should be project full name")
				} else {
					require.Fail(t, "failed to cast Subject Content")
				}
			}
		})
	}
}
