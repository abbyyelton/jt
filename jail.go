package main

import (
	"fmt"
	"log"
	"math/rand"
	"strings"
	"time"
)

/*
type Offense struct {
	Offense           string `json:"Offense"`
	ArrestCode        string `json:"Arrest Code"`
	Statute           string `json:"Statute"`
	ChargeDescription string `json:"Charge Description"`
	Status            string `json:"Status"`
	CaseNumber        string `json:"Case Number"`
	ControlNumber     string `json:"Control Number"`
	CourtDate         string `json:"CourtDate"`
	CourtType         string `json:"CourtType"`
}*/

type File struct {
	FileName string `json:"filename"`
	FileType string `json:"type"`
	Data     string `json:"data"`
}

type Prisoner struct {
	Id                        int    `json:"id"`
	AgencyOffenderId          string `json:"agencyOffenderId"`
	AgencyOffenderPermanentId string `json:"agencyOffenderPermanentId"`
	FirstName                 string `json:"firstName"`
	LastName                  string `json:"lastName"`
	MiddleName                string `json:"middleName"`
	NameSuffix                string `json:"nameSuffix"`
	Gender                    string `json:"gender"`
	SupervisionStatus         string `json:"supervisionStatus"`
	ImageUri                  string `json:"imageUri"`
	DetailsJson               string `json:"detailsJson"`
	BookDate                  string `json:"bookDate"`
	ReleaseDate               string `json:"releaseDate"`
	CreatedDateTime           string `json:"createdDateTime"`
	UpdatedDateTime           string `json:"updatedDateTime"`
	MultiAgencyName           string `json:"multiAgencyName"`
	HasImage                  bool   `json:"hasImage"`
	BirthDate                 string `json:"birthDate"`
	Ssn                       string `json:"ssn"`
	DriversLicenseNumber      string `json:"driversLicenseNumber"`
	StateId                   string `json:"stateId"`
	BirthDateEncrypted        string `json:"birthDateEncrypted"`
	SsnEncrypted              string `json:"ssnEncrypted"`
}

type FullJailResponse struct {
	// Yes, "Requred", this is in their API. This key (like others) is typo'd.
	// (Also typo'd in the inmate request, but NOT in <FACILITY>/NameSearch)
	Success bool `json:"success"`
	// They'll keep updating this
	StatusCode int `json:"statusCode"`
	// The initial list of inmate data
	ValidationErrors []string `json:"validationErrors"`
	// This is updated with every request
	Prisoners []Prisoner `json:"data"`
	// Empty string on success, non-empty on error.
	// JailTracker sitll returns a 200 for what should be an internal server error or bad gateway,
	// but this will at least be set.
	ErrorMessage string `json:"errorMessage"`
}

type JailResponse struct {
	// Yes, "Requred", this is in their API. This key (like others) is typo'd.
	// (Also typo'd in the inmate request, but NOT in <FACILITY>/NameSearch)
	CaptchaRequired bool `json:"captchaRequred"`
	// They'll keep updating this
	CaptchaKey string `json:"captchaKey"`
	// The initial list of inmate data
	Offenders []Inmate `json:"offenders"`
	// This is updated with every request
	OffenderViewKey int `json:"offenderViewKey"`
	// Empty string on success, non-empty on error.
	// JailTracker sitll returns a 200 for what should be an internal server error or bad gateway,
	// but this will at least be set.
	ErrorMessage string `json:"errorMessage"`
}

// Jail is a top-level struct for a task to retrieve the list of inmates in a jail.
// This is also the type used to serialize to JSON for storage.
type Jail struct {
	// BaseURL for the jail. Usually "https://omsweb.public-safety-cloud.com", but not always!
	BaseURL string
	// Name of the jail, as it appears in the URL
	Name string
	// This is sent with each request, and sometimes updated
	CaptchaKey string
	//TODO rename "offenders" to something more appropriate; this is JailTracker terminology
	Offenders []Inmate
	// Each request (after validation) updates this key!
	OffenderViewKey int
	// When the job started
	StartTimeUTC time.Time
	// When the job ended
	EndTimeUTC time.Time
}

// Jail is a top-level struct for a task to retrieve the list of inmates in a jail.
// This is also the type used to serialize to JSON for storage.
type Jail2 struct {
	// BaseURL for the jail. Usually "https://omsweb.public-safety-cloud.com", but not always!
	BaseURL string
	// Name of the jail, as it appears in the URL
	Name string
	// This is sent with each request, and sometimes updated
	CaptchaKey string
	//TODO rename "offenders" to something more appropriate; this is JailTracker terminology
	Offenders []Prisoner
	// Each request (after validation) updates this key!
	OffenderViewKey int
	// When the job started
	StartTimeUTC time.Time
	// When the job ended
	EndTimeUTC time.Time
}

type Payload struct {
	// This is set in the GET response, and should also be sent in the POST
	recaptchaToken *string
	// This doesn't need to be set in the POST
	supervisionStatus string
	// UserCode is null in the GET response, set for POST
	multiAgencyName string
	gender          string
}

func NewJail(baseURL, name string) (*Jail2, error) {
	j := &Jail2{
		BaseURL:      baseURL,
		Name:         name,
		StartTimeUTC: time.Now().UTC(),
	}
	if err := j.updateCaptcha(); err != nil {
		return nil, fmt.Errorf("failed to update captcha: %w", err)
	}
	log.Println("Captcha matched!")

	// Make initial request for jail data
	payload := &Payload{
		recaptchaToken:    nil,
		supervisionStatus: "Active",
		// This is normally null in this request in the web client :\
		multiAgencyName: "All",
		gender:          "",
	}
	jailResponse := &FullJailResponse{}
	url := fmt.Sprintf("%s/publicroster-api/api/%s/search-offenders", baseURL, name)
	err := PostJSON[Payload, FullJailResponse](url, nil, payload, jailResponse)

	if err != nil {
		return nil, fmt.Errorf("failed to request initial jail data: %w", err)
	}
	if jailResponse.ErrorMessage != "" {
		return nil, fmt.Errorf(`non-empty error message for jail "%s": "%s"`, name, jailResponse.ErrorMessage)
	}
	var filtered []Prisoner
	for _, o := range jailResponse.Prisoners {
		if strings.Contains(strings.ToLower(o.DetailsJson), "federal") {
			filtered = append(filtered, o)
		}
	}
	j.Offenders = filtered
	return j, nil
}

func (j *Jail2) updateCaptcha() error {
	captchaMatched := false
	var captchaKey string
	var err error
	for i := 0; i < MaxCaptchaAttempts; i++ {
		captchaKey, err = ProcessCaptcha(j)
		if err != nil {
			log.Printf("failed to solve captcha: %v", err)
			continue
		}
		captchaMatched = true
		break
	}
	if !captchaMatched {
		return fmt.Errorf("failed to match captcha after %d attempts", MaxCaptchaAttempts)
	}
	j.CaptchaKey = captchaKey
	log.Println("Captcha matched!")

	return nil
}

// UpdateInmates updates all inmates in the jail.
// Currently returns only a nil error, but reserving one here for future use.
func (j *Jail) UpdateInmates() error {
	for i := range j.Offenders {
		// Chill out for a bit to be especially gentle to their server
		// Convert time.Second (duration in nanoseconds) to float, scale to 0.5-1.5 seconds
		duration := time.Duration((0.5 + rand.Float64()) * float64(time.Second))
		time.Sleep(duration)

		inmate := &j.Offenders[i]
		/*err := inmate.Update(j)
		if err != nil {
			log.Printf("failed to update inmate \"%s\": %v", inmate.ArrestNo, err)
			continue
		}*/
		log.Printf("Updated inmate \"%s\": %s, %s Charges: %s Court Type: %s Court Date: %s Booked: %s",
			inmate.ArrestNo, inmate.SpecialLastName, inmate.SpecialFirstName, inmate.Charges[0].ChargeDescription, inmate.Charges[0].CourtType, inmate.Charges[0].CourtTime, inmate.OriginalBookDateTime,
		)
	}
	return nil
}

// Get the URL for the jail's main page, as it would be accessed by a web browser.
// Jails have their own URL within the domain, but the captcha service needs to know which jail
// the captcha corresponds to, so it looks for this URL in the Referer header.
func (j Jail2) getJailURL() string {
	return fmt.Sprintf("%s/jtclientweb/jailtracker/index/%s", j.BaseURL, j.Name)
}

// Get the URL for the jail's JSON API, which will list all inmates.
func (j Jail) getJailAPIURL() string {
	return fmt.Sprintf("%s/jtclientweb/Offender/%s", j.BaseURL, j.Name)
}

func CrawlJail(baseURL, name string) (*Jail2, error) {
	j, err := NewJail(baseURL, name)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize jail: %w", err)
	}
	log.Printf("Found %d inmates", len(j.Offenders))

	// err = j.UpdateInmates()
	if err != nil {
		return nil, fmt.Errorf("failed to update inmates: %w", err)
	}
	j.EndTimeUTC = time.Now().UTC()
	return j, nil
}
