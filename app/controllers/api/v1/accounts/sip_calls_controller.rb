class Api::V1::Accounts::SipCallsController < Api::V1::Accounts::BaseController
  before_action :set_sip_call, only: [:show, :accept, :reject, :terminate]

  rescue_from Sip::CallErrors::NotRinging,
              Sip::CallErrors::AlreadyAccepted,
              Sip::CallErrors::CallFailed,
              with: :render_failed_call

  def show; end

  def accept
    sdp_answer = params[:sdp_answer]
    return render json: { error: 'sdp_answer is required' }, status: :unprocessable_entity if sdp_answer.blank?

    @sip_call = Sip::CallService.new(sip_call: @sip_call, agent: current_user).accept(sdp_answer)
    render :show
  end

  def reject
    @sip_call = Sip::CallService.new(sip_call: @sip_call, agent: current_user).reject
    render :show
  end

  def terminate
    @sip_call = Sip::CallService.new(sip_call: @sip_call, agent: current_user).terminate
    render :show
  end

  def initiate
    conversation = current_account.conversations.find_by!(display_id: params[:conversation_id])
    return render json: { error: 'sdp_offer is required' }, status: :unprocessable_entity if params[:sdp_offer].blank?

    @sip_call = Sip::CallService.initiate(conversation: conversation, agent: current_user, sdp_offer: params[:sdp_offer])
  end

  private

  def set_sip_call
    @sip_call = SipCall.find_by!(id: params[:id], account_id: current_account.id)
    authorize @sip_call.conversation, :show?
  end

  def render_failed_call(exception)
    render json: { error: exception.message }, status: :unprocessable_entity
  end
end
