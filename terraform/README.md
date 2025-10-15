
# 접속

https://argocd-server.argocd.svc.cluster.local/

## 암호 확인하는 방법

kubectl get secret argocd-secret -n argocd -o jsonpath="{.data.admin\.password}" | base64 -d

