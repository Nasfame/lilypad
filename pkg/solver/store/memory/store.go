package store

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"sync"

	"github.com/bacalhau-project/lilypad/pkg/data"
	"github.com/bacalhau-project/lilypad/pkg/jsonl"
	"github.com/bacalhau-project/lilypad/pkg/solver/store"
)

type SolverStoreMemory struct {
	jobOfferMap      map[string]*data.JobOfferContainer
	resourceOfferMap map[string]*data.ResourceOfferContainer
	dealMap          map[string]*data.DealContainer
	resultMap        map[string]*data.Result
	matchDecisionMap map[string]*data.MatchDecision
	mutex            sync.RWMutex
	logWriters       map[string]jsonl.Writer
}

func getMatchID(resourceOffer string, jobOffer string) string {
	return fmt.Sprintf("%s-%s", resourceOffer, jobOffer)
}

func loadJSONLFile[T any](filename string) ([]*T, error) {
	if _, err := os.Stat(filename); os.IsNotExist(err) {
		return nil, nil
	}

	logfile, err := os.OpenFile(filename, os.O_RDONLY, 0644)
	if err != nil {
		return nil, err
	}
	defer logfile.Close()

	scanner := bufio.NewScanner(logfile)

	structsArray := []*T{}

	for scanner.Scan() {
		var record T
		if err := json.Unmarshal(scanner.Bytes(), &record); err != nil {
			fmt.Printf("Error unmarshalling line from %s: %v\n", filename, err.Error())
			continue
		}
		structsArray = append(structsArray, &record)
	}

	err = scanner.Err()
	if err != nil {
		return nil, err
	}

	return structsArray, nil
}

func loadJSONLMap[T any](filename string, getID func(*T) string) (map[string]*T, error) {
	structsArray, err := loadJSONLFile[T](filename)
	if err != nil {
		return nil, err
	}

	structsMap := map[string]*T{}
	for _, record := range structsArray {
		structsMap[getID(record)] = record
	}

	return structsMap, nil
}

func getJSONLFilename(kind string) string {
	return fmt.Sprintf("/var/tmp/lilypad_%s.jsonl", kind)
}

func NewSolverStoreMemory() (*SolverStoreMemory, error) {
	logWriters := make(map[string]jsonl.Writer)

	kinds := []string{"job_offers", "resource_offers", "deals", "decisions", "results"}

	jobOffers, err := loadJSONLMap[data.JobOfferContainer](getJSONLFilename("job_offers"), func(jobOffer *data.JobOfferContainer) string {
		return jobOffer.ID
	})
	if err != nil {
		return nil, err
	}

	resourceOffers, err := loadJSONLMap[data.ResourceOfferContainer](getJSONLFilename("resource_offers"), func(resourceOffer *data.ResourceOfferContainer) string {
		return resourceOffer.ID
	})
	if err != nil {
		return nil, err
	}

	deals, err := loadJSONLMap[data.DealContainer](getJSONLFilename("deals"), func(deal *data.DealContainer) string {
		return deal.ID
	})
	if err != nil {
		return nil, err
	}

	results, err := loadJSONLMap[data.Result](getJSONLFilename("results"), func(result *data.Result) string {
		return result.DealID
	})
	if err != nil {
		return nil, err
	}

	decisions, err := loadJSONLMap[data.MatchDecision](getJSONLFilename("decisions"), func(decision *data.MatchDecision) string {
		return getMatchID(decision.ResourceOffer, decision.JobOffer)
	})
	if err != nil {
		return nil, err
	}

	for _, kind := range kinds {
		logfile, err := os.OpenFile(getJSONLFilename(kind), os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0644)
		if err != nil {
			return nil, err
		}
		logWriters[kind] = jsonl.NewWriter(logfile)
	}

	return &SolverStoreMemory{
		jobOfferMap:      jobOffers,
		resourceOfferMap: resourceOffers,
		dealMap:          deals,
		resultMap:        results,
		matchDecisionMap: decisions,
		logWriters:       logWriters,
	}, nil
}

func (s *SolverStoreMemory) AddJobOffer(jobOffer data.JobOfferContainer) (*data.JobOfferContainer, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.jobOfferMap[jobOffer.ID] = &jobOffer
	s.logWriters["job_offers"].Write(jobOffer)
	return &jobOffer, nil
}

func (s *SolverStoreMemory) AddResourceOffer(resourceOffer data.ResourceOfferContainer) (*data.ResourceOfferContainer, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.resourceOfferMap[resourceOffer.ID] = &resourceOffer
	s.logWriters["resource_offers"].Write(resourceOffer)
	return &resourceOffer, nil
}

func (s *SolverStoreMemory) AddDeal(deal data.DealContainer) (*data.DealContainer, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.dealMap[deal.ID] = &deal
	s.logWriters["deals"].Write(deal)
	return &deal, nil
}

func (s *SolverStoreMemory) AddResult(result data.Result) (*data.Result, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.resultMap[result.DealID] = &result
	s.logWriters["results"].Write(result)
	return &result, nil
}

func (s *SolverStoreMemory) AddMatchDecision(resourceOffer string, jobOffer string, deal string, result bool) (*data.MatchDecision, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	id := getMatchID(resourceOffer, jobOffer)
	_, ok := s.matchDecisionMap[id]
	if ok {
		return nil, fmt.Errorf("that match already exists")
	}
	decision := &data.MatchDecision{
		ResourceOffer: resourceOffer,
		JobOffer:      jobOffer,
		Deal:          deal,
		Result:        result,
	}
	s.matchDecisionMap[id] = decision
	s.logWriters["decisions"].Write(decision)
	return decision, nil
}

func (s *SolverStoreMemory) GetJobOffers(query store.GetJobOffersQuery) ([]data.JobOfferContainer, error) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	jobOffers := []data.JobOfferContainer{}
	for _, jobOffer := range s.jobOfferMap {
		matching := true
		if query.JobCreator != "" && jobOffer.JobCreator != query.JobCreator {
			matching = false
		}
		if query.NotMatched {
			if jobOffer.DealID != "" {
				matching = false
			}
		}
		if matching {
			jobOffers = append(jobOffers, *jobOffer)
		}
	}
	return jobOffers, nil
}

func (s *SolverStoreMemory) GetResourceOffers(query store.GetResourceOffersQuery) ([]data.ResourceOfferContainer, error) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	resourceOffers := []data.ResourceOfferContainer{}
	for _, resourceOffer := range s.resourceOfferMap {
		matching := true
		if query.ResourceProvider != "" && resourceOffer.ResourceProvider != query.ResourceProvider {
			matching = false
		}
		if query.Active && !data.IsActiveAgreementState(resourceOffer.State) {
			matching = false
		}
		if query.NotMatched {
			if resourceOffer.DealID != "" {
				matching = false
			}
		}
		if matching {
			resourceOffers = append(resourceOffers, *resourceOffer)
		}
	}
	return resourceOffers, nil
}

func (s *SolverStoreMemory) GetDeals(query store.GetDealsQuery) ([]data.DealContainer, error) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	deals := []data.DealContainer{}
	queryState := uint8(0)
	if query.State != "" {
		parsedState, err := data.GetAgreementState(query.State)
		if err != nil {
			return nil, err
		}
		queryState = parsedState
	}
	for _, deal := range s.dealMap {
		matching := true
		if query.JobCreator != "" && deal.JobCreator != query.JobCreator {
			matching = false
		}
		if query.ResourceProvider != "" && deal.ResourceProvider != query.ResourceProvider {
			matching = false
		}
		if query.Mediator != "" && deal.Mediator != query.Mediator {
			matching = false
		}
		if query.State != "" && deal.State != queryState {
			matching = false
		}
		if matching {
			deals = append(deals, *deal)
		}
	}
	return deals, nil
}

func (s *SolverStoreMemory) GetJobOffer(id string) (*data.JobOfferContainer, error) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	jobOffer, ok := s.jobOfferMap[id]
	if !ok {
		return nil, nil
	}
	return jobOffer, nil
}

func (s *SolverStoreMemory) GetResourceOffer(id string) (*data.ResourceOfferContainer, error) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	resourceOffer, ok := s.resourceOfferMap[id]
	if !ok {
		return nil, nil
	}
	return resourceOffer, nil
}

func (s *SolverStoreMemory) GetDeal(id string) (*data.DealContainer, error) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	deal, ok := s.dealMap[id]
	if !ok {
		return nil, nil
	}
	return deal, nil
}

func (s *SolverStoreMemory) GetResult(id string) (*data.Result, error) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	result, ok := s.resultMap[id]
	if !ok {
		return nil, nil
	}
	return result, nil
}

func (s *SolverStoreMemory) GetMatchDecision(resourceOffer string, jobOffer string) (*data.MatchDecision, error) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	id := getMatchID(resourceOffer, jobOffer)
	decision, ok := s.matchDecisionMap[id]
	if !ok {
		return nil, nil
	}
	return decision, nil
}

func (s *SolverStoreMemory) UpdateJobOfferState(id string, dealID string, state uint8) (*data.JobOfferContainer, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	jobOffer, ok := s.jobOfferMap[id]
	if !ok {
		return nil, fmt.Errorf("job offer not found: %s", id)
	}
	jobOffer.DealID = dealID
	jobOffer.State = state
	s.jobOfferMap[id] = jobOffer
	s.logWriters["job_offers"].Write(jobOffer)
	return jobOffer, nil
}

func (s *SolverStoreMemory) UpdateResourceOfferState(id string, dealID string, state uint8) (*data.ResourceOfferContainer, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	resourceOffer, ok := s.resourceOfferMap[id]
	if !ok {
		return nil, fmt.Errorf("resource offer not found: %s", id)
	}
	resourceOffer.DealID = dealID
	resourceOffer.State = state
	s.resourceOfferMap[id] = resourceOffer
	s.logWriters["resource_offers"].Write(resourceOffer)
	return resourceOffer, nil
}

func (s *SolverStoreMemory) UpdateDealState(id string, state uint8) (*data.DealContainer, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	deal, ok := s.dealMap[id]
	if !ok {
		return nil, fmt.Errorf("deal not found: %s", id)
	}
	deal.State = state
	s.dealMap[id] = deal
	s.logWriters["deals"].Write(deal)
	return deal, nil
}

func (s *SolverStoreMemory) UpdateDealMediator(id string, mediator string) (*data.DealContainer, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	deal, ok := s.dealMap[id]
	if !ok {
		return nil, fmt.Errorf("deal not found: %s", id)
	}
	deal.Mediator = mediator
	s.dealMap[id] = deal
	s.logWriters["deals"].Write(deal)
	return deal, nil
}

func (s *SolverStoreMemory) UpdateDealTransactionsResourceProvider(id string, data data.DealTransactionsResourceProvider) (*data.DealContainer, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	deal, ok := s.dealMap[id]
	if !ok {
		return nil, fmt.Errorf("deal not found: %s", id)
	}
	txs := &deal.Transactions.ResourceProvider
	if data.Agree != "" {
		txs.Agree = data.Agree
	}
	if data.AddResult != "" {
		txs.AddResult = data.AddResult
	}
	if data.TimeoutAgree != "" {
		txs.TimeoutAgree = data.TimeoutAgree
	}
	if data.TimeoutJudgeResult != "" {
		txs.TimeoutJudgeResult = data.TimeoutJudgeResult
	}
	if data.TimeoutMediateResult != "" {
		txs.TimeoutMediateResult = data.TimeoutMediateResult
	}
	s.dealMap[id] = deal
	s.logWriters["deals"].Write(deal)
	return deal, nil
}

func (s *SolverStoreMemory) UpdateDealTransactionsJobCreator(id string, data data.DealTransactionsJobCreator) (*data.DealContainer, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	deal, ok := s.dealMap[id]
	if !ok {
		return nil, fmt.Errorf("deal not found: %s", id)
	}
	txs := &deal.Transactions.JobCreator
	if data.Agree != "" {
		txs.Agree = data.Agree
	}
	if data.AcceptResult != "" {
		txs.AcceptResult = data.AcceptResult
	}
	if data.CheckResult != "" {
		txs.CheckResult = data.CheckResult
	}
	if data.TimeoutAgree != "" {
		txs.TimeoutAgree = data.TimeoutAgree
	}
	if data.TimeoutSubmitResult != "" {
		txs.TimeoutSubmitResult = data.TimeoutSubmitResult
	}
	if data.TimeoutMediateResult != "" {
		txs.TimeoutMediateResult = data.TimeoutMediateResult
	}
	s.dealMap[id] = deal
	s.logWriters["deals"].Write(deal)
	return deal, nil
}

func (s *SolverStoreMemory) UpdateDealTransactionsMediator(id string, data data.DealTransactionsMediator) (*data.DealContainer, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	deal, ok := s.dealMap[id]
	if !ok {
		return nil, fmt.Errorf("deal not found: %s", id)
	}
	txs := &deal.Transactions.Mediator
	if data.MediationAcceptResult != "" {
		txs.MediationAcceptResult = data.MediationAcceptResult
	}
	if data.MediationRejectResult != "" {
		txs.MediationRejectResult = data.MediationRejectResult
	}
	s.dealMap[id] = deal
	s.logWriters["deals"].Write(deal)
	return deal, nil
}

func (s *SolverStoreMemory) RemoveJobOffer(id string) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	delete(s.jobOfferMap, id)
	return nil
}

func (s *SolverStoreMemory) RemoveResourceOffer(id string) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	delete(s.resourceOfferMap, id)
	return nil
}

func (s *SolverStoreMemory) GetLeaderboardData() ([]data.LeaderboardEntry, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	countPerResourceProvider := map[string]int{}

	for _, deal := range s.dealMap {
		if data.IsSuccessfulAgreementState(deal.State) {
			currentCount, ok := countPerResourceProvider[deal.ResourceProvider]
			if !ok {
				currentCount = 0
			}
			countPerResourceProvider[deal.ResourceProvider] = currentCount + 1
		}
	}

	results := []data.LeaderboardEntry{}

	for resourceProvider, count := range countPerResourceProvider {
		results = append(results, data.LeaderboardEntry{
			ResourceProvider: resourceProvider,
			JobCount:         count,
		})
	}

	return results, nil
}

// Compile-time interface check:
var _ store.SolverStore = (*SolverStoreMemory)(nil)
