package main

import (
    "fmt"
    "net/http"
    "code.google.com/p/go.net/websocket"
    "github.com/gorilla/sessions"
    "github.com/gorilla/context"
    "canopy/datalayer"
    "encoding/json"
)

var store = sessions.NewCookieStore([]byte("my_production_secret"))

func loginHandler(w http.ResponseWriter, r *http.Request) {
    w.Header().Set("Content-Type", "application/json")
    w.Header().Set("Access-Control-Allow-Origin", "http://canopy.link")
    w.Header().Set("Access-Control-Allow-Credentials", "true")

    var data map[string]interface{}
    decoder := json.NewDecoder(r.Body)
    err := decoder.Decode(&data)
    if err != nil {
        fmt.Fprintf(w, "{\"error\" : \"json_decode_failed\"}")
        return
    }

    username, ok := data["username"].(string)
    if !ok {
        fmt.Fprintf(w, "{\"error\" : \"string_username_expected\"}")
        return
    }

    password, ok := data["password"].(string)
    if !ok {
        fmt.Fprintf(w, "{\"error\" : \"string_password_expected\"}")
        return
    }

    session, _ := store.Get(r, "canopy-login-session")
    dl := datalayer.NewCassandraDatalayer()
    dl.Connect("canopy")
    _, err = dl.LookupAccountVerifyPassword(username, password)
    if err == nil {
        session.Values["logged_in_username"] = username
        err := session.Save(r, w)
        if err != nil {
            fmt.Fprintf(w, "{\"error\" : \"saving_session\"}")
            return
        }
        fmt.Fprintf(w, "{\"success\" : true}")
        return
    } else {
        fmt.Fprintf(w, "{\"error\" : \"incorrect_password\"}")
        return
    }
}

func logoutHandler(w http.ResponseWriter, r *http.Request) {
    w.Header().Set("Content-Type", "application/json")
    w.Header().Set("Access-Control-Allow-Origin", "http://canopy.link")
    w.Header().Set("Access-Control-Allow-Credentials", "true")
    session, _ := store.Get(r, "canopy-login-session")
    session.Values["logged_in_username"] = ""
    err := session.Save(r, w)
    if err != nil {
        w.WriteHeader(http.StatusInternalServerError);
        fmt.Fprintf(w, "{ \"error\" : \"could_not_logout\"");
        return;
    }
    fmt.Fprintf(w, "{ \"success\" : true }")
}

func createAccountHandler(w http.ResponseWriter, r *http.Request) {
    w.Header().Set("Content-Type", "application/json")
    w.Header().Set("Access-Control-Allow-Origin", "http://canopy.link")
    w.Header().Set("Access-Control-Allow-Credentials", "true")

    var data map[string]interface{}
    decoder := json.NewDecoder(r.Body)
    err := decoder.Decode(&data)
    if err != nil {
        w.WriteHeader(http.StatusBadRequest);
        fmt.Fprintf(w, "{\"error\" : \"json_decode_failed\"}")
        return
    }

    username, ok := data["username"].(string)
    if !ok {
        w.WriteHeader(http.StatusBadRequest);
        fmt.Fprintf(w, "{\"error\" : \"string_username_expected\"}")
        return
    }

    email, ok := data["username"].(string)
    if !ok {
        w.WriteHeader(http.StatusBadRequest);
        fmt.Fprintf(w, "{\"error\" : \"string_email_expected\"}")
        return
    }

    password, ok := data["password"].(string)
    if !ok {
        w.WriteHeader(http.StatusBadRequest);
        fmt.Fprintf(w, "{\"error\" : \"string_password_expected\"}")
        return
    }

    password_confirm, ok := data["password_confirm"].(string)
    if !ok {
        w.WriteHeader(http.StatusBadRequest);
        fmt.Fprintf(w, "{\"error\" : \"string_password_confirm_expected\"}")
        return
    }

    if (password != password_confirm) {
        w.WriteHeader(http.StatusBadRequest);
        fmt.Fprintf(w, "{\"error\" : \"passwords_dont_match\"}")
        return
    }

    dl := datalayer.NewCassandraDatalayer()
    dl.Connect("canopy")

    dl.CreateAccount(username, email, password);
    session, _ := store.Get(r, "canopy-login-session")
    session.Values["logged_in_username"] = username
    err = session.Save(r, w)
    if err != nil {
        w.WriteHeader(http.StatusInternalServerError);
        fmt.Fprintf(w, "{\"error\" : \"saving_session\"}")
        return
    }
    fmt.Fprintf(w, "{\"success\" : true}")
    return
}

func meHandler(w http.ResponseWriter, r *http.Request) {
    w.Header().Set("Content-Type", "application/json")
    w.Header().Set("Access-Control-Allow-Origin", "http://canopy.link")
    w.Header().Set("Access-Control-Allow-Credentials", "true")
    session, _ := store.Get(r, "canopy-login-session")
    
    username, ok := session.Values["logged_in_username"]
    if ok {
        username_string, ok := username.(string)
        if ok && username_string != "" {
            fmt.Fprintf(w, "{\"username\" : \"%s\"}", username_string);
            return
        } else {
            w.WriteHeader(http.StatusUnauthorized);
            fmt.Fprintf(w, "{\"error\" : \"not_logged_in\"}");
            return
        }
    } else {
        w.WriteHeader(http.StatusUnauthorized);
        fmt.Fprintf(w, "{\"error\" : \"not_logged_in\"}");
        return
    }
}

/*
{
    "devices" : [
        {
            "device_id" : UUID,
            "friendly_name"
        }
    ]
} */
type devicesResponse_Device struct {
    DeviceId string `json:"device_id"`
    FriendlyName string `json:"friendly_name"`
}
type devicesResponse struct {
    Devices []devicesResponse_Device `json:"devices"`
}

func devicesHandler(w http.ResponseWriter, r *http.Request) {
    w.Header().Set("Content-Type", "application/json")
    w.Header().Set("Access-Control-Allow-Origin", "http://canopy.link")
    w.Header().Set("Access-Control-Allow-Credentials", "true")
    session, _ := store.Get(r, "canopy-login-session")
    
    var username_string string
    username, ok := session.Values["logged_in_username"]
    if ok {
        username_string, ok = username.(string)
        if !(ok && username_string != "") {
            w.WriteHeader(http.StatusUnauthorized);
            fmt.Fprintf(w, "{\"error\" : \"not_logged_in\"");
            return
        }
    } else {
        w.WriteHeader(http.StatusUnauthorized);
        fmt.Fprintf(w, "{\"error\" : \"not_logged_in\"");
        return
    }
    
    dl := datalayer.NewCassandraDatalayer()
    dl.Connect("canopy")
    account, err := dl.LookupAccount(username_string)
    if err != nil {
        w.WriteHeader(http.StatusInternalServerError);
        fmt.Fprintf(w, "{\"error\" : \"account_lookup_failed\"}");
        return
    }

    devices, err := account.GetDevices()
    if err != nil {
        w.WriteHeader(http.StatusInternalServerError);
        fmt.Fprintf(w, "{\"error\" : \"device_lookup_failed\"}");
        return
    }

    var devResp devicesResponse

    for _, device := range devices {
        devResp.Devices = append(devResp.Devices, devicesResponse_Device{device.GetId().String(), device.GetFriendlyName()})
    }

    jsn, err := json.Marshal(devResp)
    if err != nil {
        w.WriteHeader(http.StatusInternalServerError);
        fmt.Fprintf(w, "{\"error\" : \"generating_json\"}");
        return
    }
    fmt.Fprintf(w, string(jsn))

    return 
}

func main() {
    fmt.Println("starting server");
    http.Handle("/echo", websocket.Handler(CanopyWebsocketServer))
    http.HandleFunc("/login", loginHandler)
    http.HandleFunc("/logout", logoutHandler)
    http.HandleFunc("/create_account", createAccountHandler)
    http.HandleFunc("/me", meHandler)
    http.HandleFunc("/devices", devicesHandler)
    http.ListenAndServe(":8080", context.ClearHandler(http.DefaultServeMux))
}
