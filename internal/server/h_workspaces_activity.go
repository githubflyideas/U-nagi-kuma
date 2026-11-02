package server

import (
 "net/http"
 "strconv"
)
func(s *Server)workspaceEvents(w http.ResponseWriter,r *http.Request)(any,error){
 var id int64
 if raw:=r.URL.Query().Get("monitor_id");raw!="" {var err error;id,err=strconv.ParseInt(raw,10,64);if err!=nil||id<=0{return nil,Errorf(400,CodeInvalid,"invalid monitor ID")}}
 rows,err:=s.app.WorkspaceEvents(workspaceContext(r),id,100);if err!=nil{return nil,workspaceError(err)}
 return map[string]any{"items":rows},nil
}
func(s *Server)workspaceIncidents(w http.ResponseWriter,r *http.Request)(any,error){
 rows,err:=s.app.WorkspaceIncidents(workspaceContext(r),100);if err!=nil{return nil,workspaceError(err)}
 items:=[]map[string]any{}
 for _,row:=range rows {items=append(items,map[string]any{"id":row.ID,"monitor_id":row.MonitorID,"opened_at":timeOrNull(row.State.OpenedAt),"resolved_at":timeOrNull(row.State.ResolvedAt),"closed":row.State.Closed,"delivered":row.State.Told,"notification_count":row.State.NotifyCount})}
 return map[string]any{"items":items},nil
}
