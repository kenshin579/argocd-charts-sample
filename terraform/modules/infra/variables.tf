# argocd namespace을 생성한다
variable "argocd_namespace" {
  type    = string
  default = "argocd"
}

# argocd 암호는 argocd 명령어로 생성한다
# argocd account bcrypt --password 'password'
variable "argocd_password" {
  type    = string
  default = "$2a$10$QkJXsQ9FvKBypo3KemwlDuUpj0q8Dr0qMHH4Kbvn3W5hfBIJf2gOy"
}
