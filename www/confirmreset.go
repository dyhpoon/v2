package main

import (
	"html/template"
	"net/http"
	"net/url"

	"google.golang.org/grpc/metadata"

	"github.com/Sirupsen/logrus"
	"github.com/gorilla/csrf"
	"github.com/gorilla/mux"
	"golang.org/x/net/context"
	"v2.staffjoy.com/account"
	"v2.staffjoy.com/auth"
	"v2.staffjoy.com/company"
	"v2.staffjoy.com/crypto"
	"v2.staffjoy.com/errorpages"
)

type confirmResetPage struct {
	Title        string // Used in <title>
	CSSId        string // e.g. 'careers'
	Version      string // e.g. master-1, for cachebusting
	CsrfField    template.HTML
	ErrorMessage string
	Description  string
	TemplateName string
}

func confirmResetHandler(res http.ResponseWriter, req *http.Request) {
	page := confirmResetPage{
		Title:        "Reset your Staffjoy password",
		CSSId:        "sign-up",
		CsrfField:    csrf.TemplateField(req),
		Version:      config.GetDeployVersion(),
		TemplateName: "confirmreset.tmpl",
	}

	token := mux.Vars(req)["token"]
	if len(token) == 0 {
		errorpages.NotFound(res)
		return
	}

	email, uuid, err := crypto.VerifyEmailConfirmationToken(token, signingToken)
	if err != nil {
		http.Redirect(res, req, passwordResetPath, http.StatusFound)
	}

	if req.Method == http.MethodPost {
		// update password
		password := req.FormValue("password")
		if len(password) >= 6 {
			md := metadata.New(map[string]string{auth.AuthorizationMetadata: auth.AuthorizationWWWService})
			ctx, cancel := context.WithCancel(metadata.NewContext(context.Background(), md))
			defer cancel()
			accountClient, close, err := account.NewClient()
			if err != nil {
				panic(err)
			}
			defer close()
			a, err := accountClient.Get(ctx, &account.GetAccountRequest{Uuid: uuid})
			if err != nil {
				panic(err)
			}
			a.Email = email
			a.ConfirmedAndActive = true
			_, err = accountClient.Update(ctx, a)
			if err != nil {
				panic(err)
			}

			// Update password
			_, err = accountClient.UpdatePassword(ctx, &account.UpdatePasswordRequest{Uuid: a.Uuid, Password: password})
			if err != nil {
				panic(err)
			}

			// login user
			auth.LoginUser(a.Uuid, a.Support, false, res)
			logger.WithFields(logrus.Fields{"user_uuid": a.Uuid}).Info("user activated account and logged in")

			// Smart redirection - for onboarding purposes
			companyClient, companyClose, err := company.NewClient()
			if err != nil {
				panic(err)
			}
			defer companyClose()

			w, err := companyClient.GetWorkerOf(ctx, &company.WorkerOfRequest{UserUuid: a.Uuid})
			if err != nil {
				panic(err)
			}
			admin, err := companyClient.GetAdminOf(ctx, &company.AdminOfRequest{UserUuid: a.Uuid})
			if err != nil {
				panic(err)
			}
			var destination *url.URL
			if len(admin.Companies) != 0 || a.Support {
				destination = &url.URL{Host: "app." + config.ExternalApex, Scheme: "http"}
			} else if len(w.Teams) != 0 {
				destination = &url.URL{Host: "myaccount." + config.ExternalApex, Scheme: "http"}
			} else {
				// onboard
				destination = &url.URL{Host: "www." + config.ExternalApex, Path: "/new-company/", Scheme: "http"}
			}

			http.Redirect(res, req, destination.String(), http.StatusFound)
		}
		page.ErrorMessage = "Your password must be at least 6 characters long"
	}
	err = tmpl.ExecuteTemplate(res, page.TemplateName, page)
	if err != nil {
		panic(err)
	}
}
