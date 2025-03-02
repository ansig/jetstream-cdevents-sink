package translator

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"

	"github.com/ansig/jetstream-cdevents-sink/internal/structs"
	cdevents "github.com/cdevents/sdk-go/pkg/api"
	cdeventsv04 "github.com/cdevents/sdk-go/pkg/api/v04"
)

var (
	ErrNoCommitsOnPushEvent  error = errors.New("Push event contains no new commits, will not convert to a CD Event")
	ErrMissingRequiredFields error = errors.New("Event payload is missing required fields, cannot convert to a CD Event")
	ErrUnsupportedPRAction   error = errors.New("Unsupported Gitea Pull Request action, cannot convert to a CD Event")
	ErrUnsupportedRefType    error = errors.New("Unsupported Gitea Create ref type, cannot convert to a CD Event")
)

type GiteaPush struct{}

func (g *GiteaPush) Translate(data []byte) (cdevents.CDEvent, error) {

	var giteaEvent structs.GiteaPushEvent
	if err := json.Unmarshal(data, &giteaEvent); err != nil {
		return nil, err
	}

	cdEvent, err := cdeventsv04.NewChangeMergedEvent()
	if err != nil {
		return nil, err
	}

	if err := addSourcesFromRepositoryUrl(giteaEvent, cdEvent); err != nil {
		return nil, ErrMissingRequiredFields
	}

	if giteaEvent.TotalCommits == 0 {
		return nil, ErrNoCommitsOnPushEvent
	}

	if giteaEvent.After == "" {
		return nil, ErrMissingRequiredFields
	}
	cdEvent.SetSubjectId(giteaEvent.After)

	if giteaEvent.Repository.FullName == "" {
		return nil, ErrMissingRequiredFields
	}
	cdEvent.SetSubjectRepository(&cdevents.Reference{Id: giteaEvent.Repository.FullName})

	if err := addGiteaEventAsCustomData(giteaEvent, cdEvent); err != nil {
		return nil, err
	}

	return cdEvent, nil
}

type GiteaPullRequest struct{}

func (g *GiteaPullRequest) Translate(data []byte) (cdevents.CDEvent, error) {

	var giteaEvent structs.GiteaPullRequestEvent
	if err := json.Unmarshal(data, &giteaEvent); err != nil {
		return nil, err
	}

	if giteaEvent.Repository.FullName == "" {
		return nil, ErrMissingRequiredFields
	}

	var cdEvent cdevents.CDEvent

	if giteaEvent.Action == "" {
		return nil, ErrMissingRequiredFields
	}

	switch giteaEvent.Action {
	case "opened":
		changeCreatedEvent, err := cdeventsv04.NewChangeCreatedEvent()
		if err != nil {
			return nil, err
		}
		changeCreatedEvent.SetSubjectRepository(&cdevents.Reference{Id: giteaEvent.Repository.FullName})
		if giteaEvent.PullRequest.Title == "" {
			return nil, ErrMissingRequiredFields
		}
		changeCreatedEvent.SetSubjectDescription(giteaEvent.PullRequest.Title)
		cdEvent = changeCreatedEvent
	case "closed":
		changeMergedEvent, err := cdeventsv04.NewChangeMergedEvent()
		if err != nil {
			return nil, err
		}
		changeMergedEvent.SetSubjectRepository(&cdevents.Reference{Id: giteaEvent.Repository.FullName})
		cdEvent = changeMergedEvent
	default:
		return nil, ErrUnsupportedPRAction
	}

	addSourcesFromRepositoryUrl(giteaEvent, cdEvent)

	if giteaEvent.PullRequest.Id == 0 {
		return nil, ErrMissingRequiredFields
	}
	cdEvent.SetSubjectId(fmt.Sprintf("pr-%d", giteaEvent.PullRequest.Id))
	if err := cdEvent.SetCustomData("application/json", giteaEvent); err != nil {
		return nil, err
	}

	if err := addGiteaEventAsCustomData(giteaEvent, cdEvent); err != nil {
		return nil, err
	}

	return cdEvent, nil
}

type GiteaCreate struct{}

func (g *GiteaCreate) Translate(data []byte) (cdevents.CDEvent, error) {

	var giteaEvent structs.GiteaCreateEvent
	if err := json.Unmarshal(data, &giteaEvent); err != nil {
		return nil, err
	}

	var cdEvent cdevents.CDEvent

	switch giteaEvent.RefType {
	case "branch":
		branchCreatedEvent, err := cdeventsv04.NewBranchCreatedEvent()
		if err != nil {
			return nil, err
		}
		branchCreatedEvent.SetSubjectRepository(&cdevents.Reference{Id: giteaEvent.Repository.FullName})
		cdEvent = branchCreatedEvent
	default:
		return nil, ErrUnsupportedRefType
	}

	addSourcesFromRepositoryUrl(giteaEvent, cdEvent)

	if giteaEvent.Ref == "" {
		return nil, ErrMissingRequiredFields
	}
	cdEvent.SetSubjectId(giteaEvent.Ref)

	if err := addGiteaEventAsCustomData(giteaEvent, cdEvent); err != nil {
		return nil, err
	}

	return cdEvent, nil
}

type GiteaDelete struct{}

func (g *GiteaDelete) Translate(data []byte) (cdevents.CDEvent, error) {

	var giteaEvent structs.GiteaDeleteEvent
	if err := json.Unmarshal(data, &giteaEvent); err != nil {
		return nil, err
	}

	var cdEvent cdevents.CDEvent

	switch giteaEvent.RefType {
	case "branch":
		branchDeletedEvent, err := cdeventsv04.NewBranchDeletedEvent()
		if err != nil {
			return nil, err
		}
		branchDeletedEvent.SetSubjectRepository(&cdevents.Reference{Id: giteaEvent.Repository.FullName})
		cdEvent = branchDeletedEvent
	default:
		return nil, ErrUnsupportedRefType
	}

	addSourcesFromRepositoryUrl(giteaEvent, cdEvent)

	if giteaEvent.Ref == "" {
		return nil, ErrMissingRequiredFields
	}
	cdEvent.SetSubjectId(giteaEvent.Ref)

	if err := addGiteaEventAsCustomData(giteaEvent, cdEvent); err != nil {
		return nil, err
	}

	return cdEvent, nil
}

func addGiteaEventAsCustomData(giteaEvent interface{}, cdEvent cdevents.CDEvent) error {
	customData := struct {
		Kind    string
		Content interface{}
	}{
		Kind:    fmt.Sprintf("%T", giteaEvent),
		Content: giteaEvent,
	}
	if err := cdEvent.SetCustomData("application/json", customData); err != nil {
		return err
	}
	return nil
}

func addSourcesFromRepositoryUrl(giteaEvent interface{}, cdEvent cdevents.CDEvent) error {

	var rawRepoUrl string
	switch v := giteaEvent.(type) {
	case structs.GiteaCreateEvent:
		rawRepoUrl = v.Repository.HtmlUrl
	case structs.GiteaDeleteEvent:
		rawRepoUrl = v.Repository.HtmlUrl
	case structs.GiteaPushEvent:
		rawRepoUrl = v.Repository.HtmlUrl
	case structs.GiteaPullRequestEvent:
		rawRepoUrl = v.Repository.HtmlUrl
	default:
		panic(fmt.Sprintf("failed to extract repository URL from Gitea event with type: %T", giteaEvent))
	}

	if rawRepoUrl == "" {
		return errors.New("Missing required field: repository.html_url")
	}

	repoUrl, err := url.Parse(rawRepoUrl)
	if err != nil {
		return err
	}

	cdEvent.SetSource(repoUrl.Host)

	subjectSource, err := url.JoinPath(repoUrl.Host, repoUrl.Path)
	if err != nil {
		return err
	}

	cdEvent.SetSubjectSource(subjectSource)

	return nil
}
