# 오픈 시뮬레이터

[![Go Report Card](https://goreportcard.com/badge/github.com/alibaba/open-simulator)](https://goreportcard.com/report/github.com/alibaba/open-simulator)
![workflow build](https://github.com/alibaba/open-simulator/actions/workflows/build.yml/badge.svg)

[English](./README.md) | [简体中文](./README_zh.md) | 한국어

## Introduction

Open-Simulator는 Kubernetes의 **클러스터 시뮬레이션 구성 요소**입니다. Open-Simulator의 시뮬레이션 기능을 통해 사용자는 가상 Kubernetes 클러스터를 만들고 [Workload](https://kubernetes.io/ko/docs/concepts/workloads/) 리소스를 배포할 수 있습니다. Open-Simulator는 [Kube-Controller-Manager](https://kubernetes.io/docs/reference/command-line-tools-reference/kube-controller-manager/)를 시뮬레이션하여 가상 환경에서 워크로드 리소스의 Pod 인스턴스를 생성합니다. 클러스터를 만들고 [Kube-Scheduler](https://kubernetes.io/docs/reference/command-line-tools-reference/kube-scheduler/)를 시뮬레이션하여 Pod를 예약합니다.

## Use Case

- 용량 계획: 기존 서버 사양(CPU 코어 수, 메모리, 디스크 수 포함) 및 애플리케이션 배포 파일(지정된 복사본 수, 선호도 규칙, 리소스 애플리케이션 양 포함)에 따라 필요한 클러스터 및 애플리케이션 계획 노드 수를 성공적으로 설치하려면
- 시뮬레이션 스케줄링: 실행 중인 Kubernetes 클러스터에서 배포할 애플리케이션을 한 번에 성공적으로 배포할 수 있는지 여부를 판단하고 클러스터 크기가 배포 요구 사항에 맞지 않을 경우 추가할 노드 수를 계획하여 All-or - 응용 프로그램 일정 관련 질문이 없습니다.
- 컨테이너 마이그레이션: 실행 중인 Kubernetes 클러스터에서 정책에 따라 노드 간에 Pod가 마이그레이션됩니다. 앞으로 다음 마이그레이션 전략 지원을 고려하십시오.
  - 클러스터 확장
  - 조각 모음

위의 문제를 해결함으로써 Open-Simulator는 인건비와 운영 및 유지 관리 비용을 줄이고 클러스터 리소스의 전반적인 활용도를 높일 것입니다.

## ✅ Feature

- 모든 크기의 K8 클러스터 생성 지원
- 지정된 순서로 워크로드 배포 지원
- Kube-Scheduler 스케줄링 시뮬레이션 지원 및 애플리케이션 배포 토폴로지 결과 제공
- 확장 스케줄링 알고리즘 지원
- 클러스터 자원 수위 설정 지원

## User guide

자세한 내용은 [문서](docs/user-guide/user-guide_ko_KR.md)를 참조하세요.

## LICENSE

[Apache 2.0 License](LICENSE)
