// Package seed defines the predefined synthetic personas, partner clients
// and admin users loaded into a fresh (or reset) sandbox.
//
// All names, numbers and addresses are fictional. Mobile numbers use the
// 99999xxxxx block, e-mails use the reserved .test TLD, PANs start with ZZZ,
// and driving licence / vehicle numbers use RTO code 00, which is not issued.
package seed

import (
	"time"

	"mockdigilocker/internal/catalog"
	"mockdigilocker/internal/model"
)

// DefaultPIN is the security PIN of every predefined persona.
const DefaultPIN = "123456"

type DocSpec struct {
	DocType string
	Opts    catalog.Opts
}

type Persona struct {
	User model.User
	Docs []DocSpec
}

func addr(house, street, loc, dist, state, pin, co string) model.Address {
	return model.Address{CareOf: co, House: house, Street: street, Locality: loc, VTC: dist, SubDistrict: dist, District: dist, State: state, Pincode: pin, Country: "India"}
}

// Personas returns the predefined test personas. IDs are stable so tests and
// QA scripts can reference them.
func Personas() []Persona {
	return []Persona{
		{
			User: model.User{
				ID: "a1b2c3d4-0001-4000-8000-000000000001", Username: "asha.normal", Name: "Asha Testkumari", DOB: "1990-08-15", Gender: "F",
				Mobile: "9999900001", Email: "asha.normal@sandbox.test", Persona: "normal_all_docs",
				Description: "Normal user with Aadhaar, PAN and Driving Licence (all valid)",
				Address:     addr("12 Sandbox Residency", "Mock Main Road", "Testnagar", "Mockpur", "Karnataka", "560999", "D/O Ravi Testkumar"),
				EAadhaar:    true, AadhaarLast4: "0001", PAN: "ZZZPT0001Z", DrivingLicence: "KA0020190000001", FatherName: "Ravi Testkumar",
			},
			Docs: []DocSpec{{DocType: catalog.DocAadhaar}, {DocType: catalog.DocPAN}, {DocType: catalog.DocDL}},
		},
		{
			User: model.User{
				ID: "a1b2c3d4-0002-4000-8000-000000000002", Username: "nikhil.nodocs", Name: "Nikhil Nodocs", DOB: "1995-01-20", Gender: "M",
				Mobile: "9999900002", Email: "nikhil.nodocs@sandbox.test", Persona: "no_documents",
				Description: "User with no issued documents and no e-Aadhaar",
				Address:     addr("4 Empty Lane", "Blank Street", "Nullpura", "Mockpur", "Maharashtra", "400999", "S/O Mohan Nodocs"),
				EAadhaar:    false, FatherName: "Mohan Nodocs",
			},
		},
		{
			User: model.User{
				ID: "a1b2c3d4-0003-4000-8000-000000000003", Username: "priya.partial", Name: "Priya Partialdas", DOB: "1988-11-02", Gender: "F",
				Mobile: "9999900003", Email: "priya.partial@sandbox.test", Persona: "partial_documents",
				Description: "User with Aadhaar and PAN only (no Driving Licence)",
				Address:     addr("77 Half Way House", "Partial Road", "Midtown", "Sampleganj", "Uttar Pradesh", "226999", "W/O Arun Partialdas"),
				EAadhaar:    true, AadhaarLast4: "0003", PAN: "ZZZPP0003P", FatherName: "Kishore Partialdas",
			},
			Docs: []DocSpec{{DocType: catalog.DocAadhaar}, {DocType: catalog.DocPAN}},
		},
		{
			User: model.User{
				ID: "a1b2c3d4-0004-4000-8000-000000000004", Username: "rahul.expired", Name: "Rahul Expiredwala", DOB: "1975-03-10", Gender: "M",
				Mobile: "9999900004", Email: "rahul.expired@sandbox.test", Persona: "expired_documents",
				Description: "Driving Licence and vehicle RC expired; Aadhaar and PAN valid",
				Address:     addr("9 Old Town", "Bygone Street", "Pastpur", "Mockpur", "Tamil Nadu", "600999", "S/O Suresh Expiredwala"),
				EAadhaar:    true, AadhaarLast4: "0004", PAN: "ZZZPE0004E", DrivingLicence: "TN0020050000004", FatherName: "Suresh Expiredwala",
			},
			Docs: []DocSpec{
				{DocType: catalog.DocAadhaar}, {DocType: catalog.DocPAN},
				{DocType: catalog.DocDL, Opts: catalog.Opts{IssueDate: "2005-06-01", ValidTo: "2023-05-31"}},
				{DocType: catalog.DocRC, Opts: catalog.Opts{IssueDate: "2008-02-11", ValidTo: "2023-02-10", Number: "TN00EX0004"}},
			},
		},
		{
			User: model.User{
				ID: "a1b2c3d4-0005-4000-8000-000000000005", Username: "irfan.invalid", Name: "Irfan Invalidkhan", DOB: "1992-07-07", Gender: "M",
				Mobile: "9999900005", Email: "irfan.invalid@sandbox.test", Persona: "invalid_documents",
				Description: "PAN marked invalid and Driving Licence revoked by issuer",
				Address:     addr("0 Faulty Towers", "Error Avenue", "Bugpet", "Mockpur", "Telangana", "500999", "S/O Imran Invalidkhan"),
				EAadhaar:    true, AadhaarLast4: "0005", PAN: "ZZZPI0005I", DrivingLicence: "TS0020170000005", FatherName: "Imran Invalidkhan",
			},
			Docs: []DocSpec{
				{DocType: catalog.DocAadhaar},
				{DocType: catalog.DocPAN, Opts: catalog.Opts{Status: model.DocInvalid}},
				{DocType: catalog.DocDL, Opts: catalog.Opts{Status: model.DocRevoked}},
			},
		},
		{
			User: model.User{
				ID: "a1b2c3d4-0006-4000-8000-000000000006", Username: "meera.multi", Name: "Meera Multidocs", DOB: "1985-12-25", Gender: "F",
				Mobile: "9999900006", Email: "meera.multi@sandbox.test", Persona: "multiple_same_issuer",
				Description: "Driving Licence plus two vehicle RCs from the same issuer (MoRTH)",
				Address:     addr("3 Many Gates", "Plural Road", "Copytown", "Mockpur", "Gujarat", "380999", "D/O Mahesh Multidocs"),
				EAadhaar:    true, AadhaarLast4: "0006", PAN: "ZZZPM0006M", DrivingLicence: "GJ0020150000006", FatherName: "Mahesh Multidocs",
			},
			Docs: []DocSpec{
				{DocType: catalog.DocAadhaar}, {DocType: catalog.DocPAN}, {DocType: catalog.DocDL},
				{DocType: catalog.DocRC, Opts: catalog.Opts{Number: "GJ00MK0061", Suffix: "A"}},
				{DocType: catalog.DocRC, Opts: catalog.Opts{Number: "GJ00MK0062", Suffix: "B"}},
			},
		},
		{
			User: model.User{
				ID: "a1b2c3d4-0007-4000-8000-000000000007", Username: "dev.disabled", Name: "Dev Disabledson", DOB: "1999-04-01", Gender: "T",
				Mobile: "9999900007", Email: "dev.disabled@sandbox.test", Persona: "disabled_account",
				Description: "Disabled account — login must fail",
				Address:     addr("1 Locked Street", "Closed Road", "Shutpur", "Mockpur", "Delhi", "110999", "C/O Admin"),
				EAadhaar:    true, AadhaarLast4: "0007", PAN: "ZZZPD0007D", FatherName: "Dinesh Disabledson", Disabled: true,
			},
			Docs: []DocSpec{{DocType: catalog.DocAadhaar}, {DocType: catalog.DocPAN}},
		},
		{
			User: model.User{
				ID: "a1b2c3d4-0008-4000-8000-000000000008", Username: "pooja.pullable", Name: "Pooja Pullwali", DOB: "1993-09-30", Gender: "F",
				Mobile: "9999900008", Email: "pooja.pullable@sandbox.test", Persona: "pull_documents",
				Description: "Has e-Aadhaar and issuer records for PAN and DL that are NOT yet in the locker — test Pull Document",
				Address:     addr("8 Fetch Lane", "Pull Road", "Getnagar", "Mockpur", "Kerala", "682999", "D/O Prakash Pullwala"),
				EAadhaar:    true, AadhaarLast4: "0008", PAN: "ZZZPW0008W", DrivingLicence: "KL0020210000008", FatherName: "Prakash Pullwala",
			},
		},
	}
}

// Clients returns the default partner applications.
func Clients() []model.Client {
	now := time.Now().UTC()
	return []model.Client{
		{
			ClientID: "MOCKDEMO01", ClientSecret: "mock-demo-secret-change-me", Name: "Demo Partner App",
			Description:  "Built-in demo partner used by the /demo-partner page and the Postman collection",
			RedirectURIs: []string{"http://localhost:8080/demo-partner/callback", "http://localhost:3000/callback", "https://oauth.pstmn.io/v1/callback"},
			Scopes:       []string{"openid", "files.issueddocs", "userdetails", "eaadhaar"}, Enabled: true, Profile: "digilocker",
			CreatedAt: now, UpdatedAt: now,
		},
		{
			ClientID: "MOCKPKCE01", ClientSecret: "mock-pkce-secret-change-me", Name: "PKCE Mobile App",
			Description: "Client that requires PKCE (S256)", RequirePKCE: true,
			RedirectURIs: []string{"http://localhost:8080/demo-partner/callback", "com.example.app://callback"},
			Scopes:       []string{"openid", "files.issueddocs", "userdetails"}, Enabled: true, Profile: "digilocker",
			CreatedAt: now, UpdatedAt: now,
		},
	}
}
