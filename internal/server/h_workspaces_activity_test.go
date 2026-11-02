package server

import (
 "fmt"
 "net/http"
 "net/http/httptest"
 "testing"
 "time"
 "github.com/githubflyideas/unagikuma/internal/store"
)
func TestWorkspaceHTTPActivity(t *testing.T){
 e:=newEnv(t,"@ IN SOA unagikuma. admin.localhost. ( 1 1s 500ms 300ms 1 )\n",nil);e.setup()
 target:=httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){w.WriteHeader(503)}));defer target.Close()
 tenant,_:=e.app.Store().DefaultTenant();uid,_:=e.app.Store().UserID("admin");owner:=store.TenantContext{TenantID:tenant.ID,UserID:uid}
 ws,err:=e.app.Store().CreateWorkspace(owner,"activity");if err!=nil{t.Fatal(err)}
 ctx:=store.WorkspaceContext{TenantContext:owner,WorkspaceID:ws.ID}
 raw:=fmt.Sprintf("@ IN SOA unagikuma. admin.localhost. ( 1 1s 500ms 300ms 1 )\nprivate IN HTTP %s attempts=1 timeout=1s\n",target.URL)
 if _,err=e.app.ApplyWorkspaceConfig(ctx,[]byte(raw),0);err!=nil{t.Fatal(err)}
 root:="/api/v1/companies/"+string(tenant.ID)+"/workspaces/"+string(ws.ID)
 var events struct{Items []store.WorkspaceEvent}
 deadline:=time.Now().Add(5*time.Second)
 for time.Now().Before(deadline){if code:=e.do("GET",root+"/events",nil,&events);code!=200{t.Fatalf("events %d",code)};if len(events.Items)>0{break};time.Sleep(50*time.Millisecond)}
 if len(events.Items)==0{t.Fatal("missing scoped problem event")}
 var incidents struct{Items []map[string]any}
 if code:=e.do("GET",root+"/incidents",nil,&incidents);code!=200||len(incidents.Items)!=1{t.Fatalf("incidents %d %+v",code,incidents)}
 foreign,_:=e.app.Store().CreateWorkspace(owner,"foreign")
 path:="/api/v1/companies/"+string(tenant.ID)+"/workspaces/"+string(foreign.ID)+fmt.Sprintf("/events?monitor_id=%d",events.Items[0].MonitorID)
 if code:=e.do("GET",path,nil,nil);code!=403{t.Fatalf("foreign event IDs %d",code)}
}
