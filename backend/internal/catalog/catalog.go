// Package catalog defines the mock issuers and document types, and builds
// synthetic documents for personas.
package catalog

import (
	"fmt"
	"strings"
	"time"

	"mockdigilocker/internal/model"
)

const (
	DocAadhaar = "ADHAR"
	DocPAN     = "PANCR"
	DocDL      = "DRVLC"
	DocRC      = "RVCER"
)

// Issuers returns the default mock issuers. Names are suffixed with
// "(Mock)" so they are never mistaken for the real departments.
func Issuers() []model.Issuer {
	return []model.Issuer{
		{
			OrgID: "000101", IssuerID: "in.gov.uidai", Name: "Unique Identification Authority of India (Mock)",
			Category: "Identity,Central Government", Description: "Synthetic Aadhaar issuer for sandbox testing", Available: true,
			DocTypes: []model.DocTypeDef{{DocType: DocAadhaar, Description: "Aadhaar Card", Parameters: []model.PullParam{}}},
		},
		{
			OrgID: "000102", IssuerID: "in.gov.pan", Name: "Income Tax Department (Mock)",
			Category: "Identity,Central Government", Description: "Synthetic PAN issuer for sandbox testing", Available: true,
			DocTypes: []model.DocTypeDef{{DocType: DocPAN, Description: "PAN Verification Record", Parameters: []model.PullParam{
				{Label: "PAN", ParamName: "panno", Example: "ZZZPT0001Z"},
				{Label: "Full Name (as on PAN)", ParamName: "PANFullName", Example: "Asha Testkumari"},
			}}},
		},
		{
			OrgID: "000103", IssuerID: "in.gov.transport", Name: "Ministry of Road Transport and Highways (Mock)",
			Category: "Transport,Central Government", Description: "Synthetic driving licence and vehicle registration issuer", Available: true,
			DocTypes: []model.DocTypeDef{
				{DocType: DocDL, Description: "Driving License", Parameters: []model.PullParam{
					{Label: "Driving Licence Number", ParamName: "dlno", Example: "KA0020190000001"},
					{Label: "Date of Birth (DD-MM-YYYY)", ParamName: "DOB", Example: "15-08-1990"},
				}},
				{DocType: DocRC, Description: "Registration of Vehicles", Parameters: []model.PullParam{
					{Label: "Vehicle Registration Number", ParamName: "regno", Example: "KA00MK0001"},
				}},
			},
		},
	}
}

func FindIssuer(issuers []model.Issuer, key string) (*model.Issuer, *model.DocTypeDef) {
	for i := range issuers {
		is := &issuers[i]
		if is.OrgID == key || is.IssuerID == key {
			return is, nil
		}
		for j := range is.DocTypes {
			if is.DocTypes[j].DocType == key {
				return is, &is.DocTypes[j]
			}
		}
	}
	return nil, nil
}

// URI builds a DigiLocker-style document URI: <issuerid>-<doctype>-<docid>.
func URI(issuerID, docType, docNumber string) string {
	id := strings.Map(func(r rune) rune {
		if (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			return r
		}
		return -1
	}, docNumber)
	return fmt.Sprintf("%s-%s-%s", issuerID, docType, id)
}

// Opts tweaks generated documents.
type Opts struct {
	Status    string // default valid
	IssueDate string
	ValidTo   string
	Suffix    string // disambiguates multiple docs of one type
	Number    string // overrides the document number
}

// Build creates a synthetic document of docType for user u.
func Build(u *model.User, docType string, o Opts) (*model.Document, error) {
	issuer, dt := FindIssuer(Issuers(), docType)
	if issuer == nil || dt == nil {
		return nil, fmt.Errorf("unknown doctype %q", docType)
	}
	now := time.Now().UTC()
	d := &model.Document{
		UserID: u.ID, DocType: docType, IssuerID: issuer.IssuerID, Issuer: shortIssuer(issuer.IssuerID), OrgID: issuer.OrgID,
		Description: dt.Description, Status: model.DocValid, Mime: []string{"application/pdf", "application/xml"},
		Fields: map[string]string{"name": u.Name, "dob": u.DOB, "gender": u.Gender}, CreatedAt: now, UpdatedAt: now,
	}
	if o.Status != "" {
		d.Status = o.Status
	}
	d.IssueDate = firstNonEmpty(o.IssueDate, "2020-01-15")
	addr := FormatAddress(u.Address)
	switch docType {
	case DocAadhaar:
		last4 := firstNonEmpty(u.AadhaarLast4, "0000")
		d.DocNumber = firstNonEmpty(o.Number, "XXXX XXXX "+last4)
		d.Name = "Aadhaar Card"
		d.Fields["masked_aadhaar"] = "XXXX XXXX " + last4
		d.Fields["address"] = addr
		d.Fields["care_of"] = u.Address.CareOf
	case DocPAN:
		d.DocNumber = firstNonEmpty(o.Number, u.PAN)
		d.Name = "PAN Verification Record"
		d.Fields["pan"] = d.DocNumber
		d.Fields["father_name"] = u.FatherName
	case DocDL:
		d.DocNumber = firstNonEmpty(o.Number, u.DrivingLicence)
		d.Name = "Driving License"
		d.ValidFrom = d.IssueDate
		d.ValidTo = firstNonEmpty(o.ValidTo, "2040-01-14")
		d.Fields["dl_no"] = d.DocNumber
		d.Fields["father_name"] = u.FatherName
		d.Fields["address"] = addr
		d.Fields["cov"] = "LMV, MCWG"
		d.Fields["rto"] = "RTO MOCKPUR (Sandbox)"
		d.Fields["blood_group"] = "O+"
	case DocRC:
		d.DocNumber = firstNonEmpty(o.Number, "KA00MK"+fmt.Sprintf("%04d", 1+len(o.Suffix)))
		d.Name = "Vehicle Registration Certificate"
		d.ValidFrom = d.IssueDate
		d.ValidTo = firstNonEmpty(o.ValidTo, "2035-01-14")
		d.Fields["reg_no"] = d.DocNumber
		d.Fields["owner"] = u.Name
		d.Fields["address"] = addr
		d.Fields["maker_model"] = "MockMotors Sandbox " + firstNonEmpty(o.Suffix, "1.2")
		d.Fields["fuel"] = "PETROL"
		d.Fields["chassis_no"] = "MOCK" + strings.ToUpper(o.Suffix) + "XXXXXXXX0001"
	}
	if o.ValidTo != "" {
		d.ValidTo = o.ValidTo
	}
	if d.DocNumber == "" {
		return nil, fmt.Errorf("persona has no identifier for %s", docType)
	}
	d.URI = URI(issuer.IssuerID, docType, d.DocNumber+o.Suffix)
	if docType == DocAadhaar {
		// Aadhaar URIs must not embed the number; use the user id instead.
		d.URI = URI(issuer.IssuerID, docType, strings.ReplaceAll(u.ID, "-", "")[:16])
	}
	return d, nil
}

func shortIssuer(id string) string {
	switch id {
	case "in.gov.uidai":
		return "UIDAI (Mock)"
	case "in.gov.pan":
		return "Income Tax Department (Mock)"
	case "in.gov.transport":
		return "MoRTH (Mock)"
	}
	return id
}

func FormatAddress(a model.Address) string {
	parts := []string{a.House, a.Street, a.Landmark, a.Locality, a.VTC, a.District, a.State}
	var out []string
	for _, p := range parts {
		if p != "" {
			out = append(out, p)
		}
	}
	s := strings.Join(out, ", ")
	if a.Pincode != "" {
		s += " - " + a.Pincode
	}
	return s
}

func firstNonEmpty(v ...string) string {
	for _, s := range v {
		if s != "" {
			return s
		}
	}
	return ""
}

// FieldLabels gives display order and labels per doctype.
func FieldLabels(docType string) [][2]string {
	common := [][2]string{{"name", "Name"}, {"dob", "Date of Birth"}, {"gender", "Gender"}}
	switch docType {
	case DocAadhaar:
		return append([][2]string{{"masked_aadhaar", "Aadhaar Number (masked)"}}, append(common, [2]string{"care_of", "Care Of"}, [2]string{"address", "Address"})...)
	case DocPAN:
		return append([][2]string{{"pan", "Permanent Account Number"}}, append(common, [2]string{"father_name", "Father's Name"})...)
	case DocDL:
		return append([][2]string{{"dl_no", "Licence Number"}}, append(common, [2]string{"father_name", "Son/Daughter/Wife of"}, [2]string{"address", "Address"}, [2]string{"cov", "Class of Vehicle"}, [2]string{"blood_group", "Blood Group"}, [2]string{"rto", "Issuing Authority"})...)
	case DocRC:
		return [][2]string{{"reg_no", "Registration Number"}, {"owner", "Owner"}, {"address", "Address"}, {"maker_model", "Maker / Model"}, {"fuel", "Fuel"}, {"chassis_no", "Chassis Number"}}
	}
	return common
}
