# VSO 가 member-api 설정 시크릿을 Kubernetes Secret 으로 동기화할 때 쓰는 권한: 설정 값 읽기만 한다
path "secret/data/member-api/config" {
  capabilities = ["read"]
}
